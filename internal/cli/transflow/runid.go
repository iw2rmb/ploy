package transflow

import (
    "fmt"
    "time"
)

func PlannerRunID(workflowID string) string {
    return fmt.Sprintf("%s-planner-%d", workflowID, time.Now().Unix())
}

func ReducerRunID(planID string) string {
    return fmt.Sprintf("%s-reducer-%d", planID, time.Now().Unix())
}

func LLMRunID(branchID string) string {
    return fmt.Sprintf("llm-exec-%s-%d", branchID, time.Now().Unix())
}

func ORWRunID(stepOrBranchID string) string {
    return fmt.Sprintf("orw-apply-%s-%d", stepOrBranchID, time.Now().Unix())
}

