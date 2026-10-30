package main

import _ "embed"

// This reviewed adaptation is bundled with the binary, never fetched at runtime.
//
//go:embed skills/finance-idea-generation/SKILL.md
var financeIdeaSkill string

const financeSkillSource = "https://github.com/anthropics/financial-services/blob/60565064557ca77b12f62aa7877f365d111c129d/plugins/vertical-plugins/equity-research/skills/idea-generation/SKILL.md"
const financeSkillRevision = "60565064557ca77b12f62aa7877f365d111c129d"

type researchSkill struct {
	Name             string `json:"name"`
	SourceURL        string `json:"source_url"`
	UpstreamRevision string `json:"upstream_revision"`
}

func activeFinanceSkill() researchSkill {
	return researchSkill{Name: "finance-idea-generation", SourceURL: financeSkillSource, UpstreamRevision: financeSkillRevision}
}
