// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package appsec

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"

	internal "github.com/DataDog/appsec-internal-go/appsec"
	rc "github.com/DataDog/datadog-agent/pkg/remoteconfig/state"

	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/orchestrion"
	"github.com/DataDog/dd-trace-go/v2/internal/remoteconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

func genApplyStatus(ack bool, err error) rc.ApplyStatus {
	status := rc.ApplyStatus{
		State: rc.ApplyStateUnacknowledged,
	}
	if err != nil {
		status.State = rc.ApplyStateError
		status.Error = err.Error()
	} else if ack {
		status.State = rc.ApplyStateAcknowledged
	}

	return status
}

func statusesFromUpdate(u remoteconfig.ProductUpdate, ack bool, err error) map[string]rc.ApplyStatus {
	statuses := make(map[string]rc.ApplyStatus, len(u))
	for path := range u {
		statuses[path] = genApplyStatus(ack, err)
	}
	return statuses
}

// combineRCRulesUpdates updates the state of the given RulesManager with the combination of all the provided rules updates
func combineRCRulesUpdates(r *config.RulesManager, updates map[string]remoteconfig.ProductUpdate) (statuses map[string]rc.ApplyStatus, err error) {
	// Spare some re-allocations (but there may still be some because 1 update may contain N configs)
	statuses = make(map[string]rc.ApplyStatus, len(updates))
	// Set the default statuses for all updates to unacknowledged
	for _, u := range updates {
		maps.Copy(statuses, statusesFromUpdate(u, false, nil))
	}

updateLoop:
	// Process rules related updates
	for p, u := range updates {
		if u != nil && len(u) == 0 {
			continue
		}
		switch p {
		case rc.ProductASMData:
			// Merge all rules data entries together and store them as a RulesManager edit entry
			fragment, status := mergeASMDataUpdates(u)
			maps.Copy(statuses, status)
			r.AddEdit("asmdata", fragment)
		case rc.ProductASMDD:
			var (
				removalFound = false
				newBasePath  string
				newBaseData  []byte
			)
			for path, data := range u {
				if data == nil && removalFound {
					err = errors.New("more than one config removal received for ASM_DD")
				} else if data != nil && newBaseData != nil {
					err = errors.New("more than one config switch received for ASM_DD")
				}
				// Already seen a removal or an update, return an error
				if err != nil {
					maps.Copy(statuses, statusesFromUpdate(u, true, err))
					break updateLoop
				}

				if data == nil {
					removalFound = true
					continue
				}

				// Save the new base path and data and only make the update after cycle through all received configs
				// This makes sure that we don't update the ruleset in case the update is invalid (ex: several non nil configs)
				newBasePath = path
				newBaseData = data
			}
			// update with data = nil means the config was removed, so we switch back to the default rules
			// only happens if no update was found, otherwise it could revert the switch to the new base rules
			if newBaseData == nil {
				if removalFound {
					log.Debug("appsec: Remote config: ASM_DD config removed. Switching back to default rules")
					r.ChangeBase(config.DefaultRulesFragment(), "")
					maps.Copy(statuses, statusesFromUpdate(u, true, nil))
				}
				continue
			}

			// Switch the base rules of the RulesManager if the config received through ASM_DD is valid
			var newBase config.RulesFragment
			if err := json.Unmarshal(newBaseData, &newBase); err != nil {
				log.Debug("appsec: Remote config: could not unmarshall ASM_DD rules: %v", err)
				statuses[newBasePath] = genApplyStatus(true, err)
				break updateLoop
			}
			log.Debug("appsec: Remote config: switching to %s as the base rules file", newBasePath)
			r.ChangeBase(newBase, newBasePath)
			statuses[newBasePath] = genApplyStatus(true, nil)
		case rc.ProductASM:
			// Store each config received through ASM as an edit entry in the RulesManager
			// Those entries will get merged together when the final rules are compiled
			// If a config gets removed, the RulesManager edit entry gets removed as well
			for path, data := range u {
				log.Debug("appsec: Remote config: processing the %s ASM config", path)
				if data == nil {
					log.Debug("appsec: Remote config: ASM config %s was removed", path)
					r.RemoveEdit(path)
					continue
				}
				var f config.RulesFragment
				if err = json.Unmarshal(data, &f); err != nil {
					log.Debug("appsec: Remote config: error processing ASM config %s: %v", path, err)
					statuses[path] = genApplyStatus(true, err)
					break updateLoop
				}
				r.AddEdit(path, f)
			}
		default:
			log.Debug("appsec: Remote config: ignoring unsubscribed product %s", p)
		}
	}

	// Set all statuses to ack if no error occured
	if err == nil {
		for _, u := range updates {
			maps.Copy(statuses, statusesFromUpdate(u, true, nil))
		}
	}

	return statuses, err

}

// onRemoteActivation is the RC callback called when an update is received for ASM_FEATURES
func (a *appsec) onRemoteActivation(updates map[string]remoteconfig.ProductUpdate) map[string]rc.ApplyStatus {
	statuses := map[string]rc.ApplyStatus{}
	if u, ok := updates[rc.ProductASMFeatures]; ok {
		statuses = a.handleASMFeatures(u)
	}
	return statuses

}

// onRCRulesUpdate is the RC callback called when security rules related RC updates are available
func (a *appsec) onRCRulesUpdate(updates map[string]remoteconfig.ProductUpdate) map[string]rc.ApplyStatus {
	// If appsec was deactivated through RC, stop here
	if !a.started {
		return map[string]rc.ApplyStatus{}
	}

	// Create a new local RulesManager
	r := a.cfg.RulesManager.Clone()
	statuses, err := combineRCRulesUpdates(&r, updates)
	if err != nil {
		log.Debug("appsec: Remote config: not applying any updates because of error: %v", err)
		return statuses
	}

	// Compile the final rules once all updates have been processed and no error occurred
	r.Compile()
	log.Debug("appsec: Remote config: final compiled rules: %s", r.String())

	// Replace the RulesManager with the new one holding the new state
	a.cfg.RulesManager = &r

	// If an error occurs while updating the WAF handle, don't swap the RulesManager and propagate the error
	// to all config statuses since we can't know which config is the faulty one
	if err = a.SwapRootOperation(); err != nil {
		log.Error("appsec: Remote config: could not apply the new security rules: %v", err)
		for k := range statuses {
			statuses[k] = genApplyStatus(true, err)
		}
		return statuses
	}

	return statuses
}

// handleASMFeatures deserializes an ASM_FEATURES configuration received through remote config
// and starts/stops appsec accordingly.
func (a *appsec) handleASMFeatures(u remoteconfig.ProductUpdate) map[string]rc.ApplyStatus {
	statuses := statusesFromUpdate(u, false, nil)
	if l := len(u); l > 1 {
		log.Error("appsec: Remote config: %d configs received for ASM_FEATURES. Expected one at most, returning early", l)
		return statuses
	}
	for path, raw := range u {
		var data rc.ASMFeaturesData
		status := rc.ApplyStatus{State: rc.ApplyStateAcknowledged}
		var err error
		log.Debug("appsec: Remote config: processing %s", path)

		// A nil config means ASM was disabled, and we stopped receiving the config file
		// Don't ack the config in this case and return early
		if raw == nil {
			log.Debug("appsec: Remote config: Stopping AppSec")
			a.stop()
			return statuses
		}
		if err = json.Unmarshal(raw, &data); err != nil {
			log.Error("appsec: Remote config: error while unmarshalling %s: %v. Configuration won't be applied.", path, err)
		} else if data.ASM.Enabled && !a.started {
			log.Debug("appsec: Remote config: Starting AppSec")
			if err = a.start(); err != nil {
				log.Error("appsec: Remote config: error while processing %s. Configuration won't be applied: %v", path, err)
				continue
			}
			registerAppsecStartTelemetry(config.ForcedOn, telemetry.OriginRemoteConfig)
		} else if !data.ASM.Enabled && a.started {
			log.Debug("appsec: Remote config: Stopping AppSec")
			a.stop()
		}
		if err != nil {
			status = genApplyStatus(false, err)
		}
		statuses[path] = status
	}

	return statuses
}

func mergeASMDataUpdates(u remoteconfig.ProductUpdate) (config.RulesFragment, map[string]rc.ApplyStatus) {
	// Following the RFC, merging should only happen when two rules data with the same ID and same Type are received
	type mapKey struct {
		id  string
		typ string
	}
	mergedRulesData := make(map[mapKey]config.DataEntry)
	mergedExclusionData := make(map[mapKey]config.DataEntry)
	statuses := statusesFromUpdate(u, true, nil)

	mergeUpdateEntry := func(mergeMap map[mapKey]config.DataEntry, data []config.DataEntry) {
		for _, ruleData := range data {
			key := mapKey{id: ruleData.ID, typ: ruleData.Type}
			if data, ok := mergeMap[key]; ok {
				// Merge rules data entries with the same ID and Type
				mergeMap[key] = config.DataEntry{
					ID:   data.ID,
					Type: data.Type,
					Data: mergeRulesDataEntries(data.Data, ruleData.Data),
				}
				continue
			}

			mergeMap[key] = ruleData
		}
	}

	mapValues := func(m map[mapKey]config.DataEntry) []config.DataEntry {
		values := make([]config.DataEntry, 0, len(m))
		for _, v := range m {
			values = append(values, v)
		}
		return values
	}

	for path, raw := range u {
		log.Debug("appsec: Remote config: processing %s", path)

		// A nil config means ASM_DATA was disabled, and we stopped receiving the config file
		// Don't ack the config in this case
		if raw == nil {
			log.Debug("appsec: remote config: %s disabled", path)
			statuses[path] = genApplyStatus(false, nil)
			continue
		}

		var asmdataUpdate struct {
			RulesData     []config.DataEntry `json:"rules_data,omitempty"`
			ExclusionData []config.DataEntry `json:"exclusion_data,omitempty"`
		}
		if err := json.Unmarshal(raw, &asmdataUpdate); err != nil {
			log.Debug("appsec: Remote config: error while unmarshalling payload for %s: %v. Configuration won't be applied.", path, err)
			statuses[path] = genApplyStatus(false, err)
			continue
		}

		mergeUpdateEntry(mergedExclusionData, asmdataUpdate.ExclusionData)
		mergeUpdateEntry(mergedRulesData, asmdataUpdate.RulesData)
	}

	var fragment config.RulesFragment
	if len(mergedRulesData) > 0 {
		fragment.RulesData = mapValues(mergedRulesData)
	}

	if len(mergedExclusionData) > 0 {
		fragment.ExclusionData = mapValues(mergedExclusionData)
	}

	return fragment, statuses
}

// mergeRulesDataEntries merges two slices of rules data entries together, removing duplicates and
// only keeping the longest expiration values for similar entries.
func mergeRulesDataEntries(entries1, entries2 []rc.ASMDataRuleDataEntry) []rc.ASMDataRuleDataEntry {
	// There will be at most len(entries1) + len(entries2)  entries in the merge map
	mergeMap := make(map[string]int64, len(entries1)+len(entries2))

	for _, entry := range entries1 {
		mergeMap[entry.Value] = entry.Expiration
	}
	// Replace the entry only if the new expiration timestamp goes later than the current one
	// If no expiration timestamp was provided (default to 0), then the data doesn't expire
	for _, entry := range entries2 {
		if exp, ok := mergeMap[entry.Value]; !ok || entry.Expiration == 0 || entry.Expiration > exp {
			mergeMap[entry.Value] = entry.Expiration
		}
	}
	// Create the final slice and return it
	entries := make([]rc.ASMDataRuleDataEntry, 0, len(mergeMap))
	for val, exp := range mergeMap {
		entries = append(entries, rc.ASMDataRuleDataEntry{Value: val, Expiration: exp})
	}
	return entries
}

func (a *appsec) startRC() error {
	if a.cfg.RC != nil {
		return remoteconfig.Start(*a.cfg.RC)
	}
	return nil
}

func (a *appsec) stopRC() {
	if a.cfg.RC != nil {
		remoteconfig.Stop()
	}
}

func (a *appsec) registerRCProduct(p string) error {
	if a.cfg.RC == nil {
		return fmt.Errorf("no valid remote configuration client")
	}
	return remoteconfig.RegisterProduct(p)
}

func (a *appsec) registerRCCapability(c remoteconfig.Capability) error {
	if a.cfg.RC == nil {
		return fmt.Errorf("no valid remote configuration client")
	}
	return remoteconfig.RegisterCapability(c)
}

func (a *appsec) unregisterRCCapability(c remoteconfig.Capability) error {
	if a.cfg.RC == nil {
		log.Debug("appsec: Remote config: no valid remote configuration client")
		return nil
	}
	return remoteconfig.UnregisterCapability(c)
}

func (a *appsec) enableRemoteActivation() error {
	if a.cfg.RC == nil {
		return fmt.Errorf("no valid remote configuration client")
	}
	err := a.registerRCProduct(rc.ProductASMFeatures)
	if err != nil {
		return err
	}
	err = a.registerRCCapability(remoteconfig.ASMActivation)
	if err != nil {
		return err
	}
	return remoteconfig.RegisterCallback(a.onRemoteActivation)
}

var blockingCapabilities = [...]remoteconfig.Capability{
	remoteconfig.ASMUserBlocking,
	remoteconfig.ASMRequestBlocking,
	remoteconfig.ASMIPBlocking,
	remoteconfig.ASMDDRules,
	remoteconfig.ASMExclusions,
	remoteconfig.ASMCustomRules,
	remoteconfig.ASMCustomBlockingResponse,
	remoteconfig.ASMTrustedIPs,
	remoteconfig.ASMExclusionData,
	remoteconfig.ASMEndpointFingerprinting,
	remoteconfig.ASMSessionFingerprinting,
	remoteconfig.ASMNetworkFingerprinting,
	remoteconfig.ASMHeaderFingerprinting,
}

func (a *appsec) enableRCBlocking() {
	if a.cfg.RC == nil {
		log.Debug("appsec: Remote config: no valid remote configuration client")
		return
	}
	if _, isSet := os.LookupEnv(internal.EnvRules); isSet {
		log.Debug("appsec: Remote config: using rules from %s, blocking capabilities won't be enabled", a.cfg.RulesManager.BasePath)
		return
	}

	products := []string{rc.ProductASM, rc.ProductASMDD, rc.ProductASMData}
	for _, p := range products {
		if err := a.registerRCProduct(p); err != nil {
			log.Debug("appsec: Remote config: couldn't register product %s: %v", p, err)
		}
	}

	if err := remoteconfig.RegisterCallback(a.onRCRulesUpdate); err != nil {
		log.Debug("appsec: Remote config: couldn't register callback: %v", err)
	}

	for _, c := range blockingCapabilities {
		if err := a.registerRCCapability(c); err != nil {
			log.Debug("appsec: Remote config: couldn't register capability %v: %v", c, err)
		}
	}
}

func (a *appsec) enableRASP() {
	if !a.cfg.RASP {
		return
	}
	if err := remoteconfig.RegisterCapability(remoteconfig.ASMRASPSSRF); err != nil {
		log.Debug("appsec: Remote config: couldn't register RASP SSRF: %v", err)
	}
	if err := remoteconfig.RegisterCapability(remoteconfig.ASMRASPSQLI); err != nil {
		log.Debug("appsec: Remote config: couldn't register RASP SQLI: %v", err)
	}
	if orchestrion.Enabled() {
		if err := remoteconfig.RegisterCapability(remoteconfig.ASMRASPLFI); err != nil {
			log.Debug("appsec: Remote config: couldn't register RASP LFI: %v", err)
		}
	}
}

func (a *appsec) disableRCBlocking() {
	if a.cfg.RC == nil {
		return
	}
	for _, c := range blockingCapabilities {
		if err := a.unregisterRCCapability(c); err != nil {
			log.Debug("appsec: Remote config: couldn't unregister capability %v: %v", c, err)
		}
	}
	if err := remoteconfig.UnregisterCallback(a.onRCRulesUpdate); err != nil {
		log.Debug("appsec: Remote config: couldn't unregister callback: %v", err)
	}
}
