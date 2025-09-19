package mods

import (
	"fmt"
	"sort"
	"time"
)

func (cj *CompactionJob) applyRetentionPolicy(cases []*CaseRecord) []*CaseRecord {
	cutoffTime := time.Now().Add(-cj.config.MaxCaseAge)
	var retained []*CaseRecord

	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Timestamp.After(cases[j].Timestamp)
	})

	kept := 0
	for _, caseRecord := range cases {
		if kept < cj.config.MinCasesToRetain {
			retained = append(retained, caseRecord)
			kept++
			continue
		}

		if caseRecord.Timestamp.After(cutoffTime) {
			retained = append(retained, caseRecord)
			kept++
		}
	}

	return retained
}

func (cj *CompactionJob) findDuplicateCases(cases []*CaseRecord) [][]int {
	var duplicateGroups [][]int
	processed := make(map[int]bool)

	for i, case1 := range cases {
		if processed[i] {
			continue
		}

		group := []int{i}
		processed[i] = true

		for j := i + 1; j < len(cases); j++ {
			if processed[j] {
				continue
			}

			case2 := cases[j]
			if cj.areCasesSimilar(case1, case2) {
				group = append(group, j)
				processed[j] = true
			}
		}

		if len(group) > 1 {
			duplicateGroups = append(duplicateGroups, group)
		}
	}

	return duplicateGroups
}

func (cj *CompactionJob) areCasesSimilar(case1, case2 *CaseRecord) bool {
	if case1.Language != case2.Language || case1.Signature != case2.Signature {
		return false
	}

	if case1.Attempt == nil || case2.Attempt == nil {
		return case1.Attempt == case2.Attempt
	}

	if case1.Attempt.Type != case2.Attempt.Type {
		return false
	}

	if case1.Attempt.Type == "orw_recipe" {
		return case1.Attempt.Recipe == case2.Attempt.Recipe
	}

	if case1.Attempt.Type == "llm_patch" {
		if case1.Attempt.PatchFingerprint == case2.Attempt.PatchFingerprint {
			return true
		}

		if case1.Attempt.PatchContent != "" && case2.Attempt.PatchContent != "" {
			similarity := cj.sigGenerator.ComputePatchSimilarity(
				[]byte(case1.Attempt.PatchContent),
				[]byte(case2.Attempt.PatchContent),
			)
			return similarity >= cj.config.SimilarityThresholdForMerge
		}
	}

	return cj.areContextsSimilar(case1.Context, case2.Context)
}

func (cj *CompactionJob) areContextsSimilar(ctx1, ctx2 *CaseContext) bool {
	if ctx1 == nil || ctx2 == nil {
		return ctx1 == ctx2
	}

	if ctx1.Language != ctx2.Language || ctx1.Lane != ctx2.Lane {
		return false
	}

	if ctx1.RepoURL != ctx2.RepoURL {
		return false
	}

	if ctx1.BuildCommand != ctx2.BuildCommand {
		return false
	}

	if ctx1.CompilerVersion != ctx2.CompilerVersion {
		return false
	}

	return true
}

func (cj *CompactionJob) mergeDuplicateCases(cases []*CaseRecord, groups [][]int) ([]*CaseRecord, int) {
	if len(groups) == 0 {
		return cases, 0
	}

	result := make([]*CaseRecord, 0, len(cases))
	toSkip := make(map[int]bool)
	totalMerged := 0

	for _, group := range groups {
		if len(group) <= 1 {
			continue
		}

		mergedCase := cj.mergeCaseGroup(cases, group)
		result = append(result, mergedCase)

		for i := 1; i < len(group); i++ {
			toSkip[group[i]] = true
		}

		totalMerged += len(group) - 1
	}

	for i, caseRecord := range cases {
		if !toSkip[i] {
			result = append(result, caseRecord)
		}
	}

	return result, totalMerged
}

func (cj *CompactionJob) mergeCaseGroup(cases []*CaseRecord, groupIndices []int) *CaseRecord {
	sort.Slice(groupIndices, func(i, j int) bool {
		return cases[groupIndices[i]].Timestamp.After(cases[groupIndices[j]].Timestamp)
	})

	baseCase := *cases[groupIndices[0]]

	if baseCase.Context == nil {
		baseCase.Context = &CaseContext{}
	}
	if baseCase.Context.Metadata == nil {
		baseCase.Context.Metadata = make(map[string]string)
	}

	baseCase.Context.Metadata["merged_cases"] = fmt.Sprintf("%d", len(groupIndices))
	baseCase.Context.Metadata["merge_timestamp"] = time.Now().Format(time.RFC3339)
	baseCase.RunID = baseCase.RunID + "_merged_" + fmt.Sprintf("%d", time.Now().Unix())

	return &baseCase
}
