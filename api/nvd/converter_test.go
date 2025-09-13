package nvd

import (
	"testing"
	"time"
)

func TestConvertToCVEInfo_UsesV31AndEnglishDescription(t *testing.T) {
	db := NewNVDDatabase()
	v := NVDVulnerability{}
	v.CVE.ID = "CVE-2024-0001"
	v.CVE.SourceIdentifier = "NVD"
	v.CVE.VulnStatus = "Analyzed"
	v.CVE.Published = "2024-01-02T03:04:05.000Z"
	v.CVE.LastModified = "2024-02-02T03:04:05.000Z"
	v.CVE.Descriptions = []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	}{{Lang: "fr", Value: "desc fr"}, {Lang: "en", Value: "desc en"}}
	v.CVE.References = []struct {
		URL    string   `json:"url"`
		Source string   `json:"source"`
		Tags   []string `json:"tags,omitempty"`
	}{{URL: "https://example.com", Source: "vendor", Tags: []string{"Vendor Advisory"}}}
	v.CVE.Configurations = []struct {
		Nodes []struct {
			Operator string `json:"operator"`
			Negate   bool   `json:"negate"`
			CpeMatch []struct {
				Vulnerable            bool   `json:"vulnerable"`
				Criteria              string `json:"criteria"`
				VersionStartIncluding string `json:"versionStartIncluding,omitempty"`
				VersionStartExcluding string `json:"versionStartExcluding,omitempty"`
				VersionEndIncluding   string `json:"versionEndIncluding,omitempty"`
				VersionEndExcluding   string `json:"versionEndExcluding,omitempty"`
				MatchCriteriaId       string `json:"matchCriteriaId"`
			} `json:"cpeMatch"`
		} `json:"nodes"`
	}{
		{Nodes: []struct {
			Operator string `json:"operator"`
			Negate   bool   `json:"negate"`
			CpeMatch []struct {
				Vulnerable            bool   `json:"vulnerable"`
				Criteria              string `json:"criteria"`
				VersionStartIncluding string `json:"versionStartIncluding,omitempty"`
				VersionStartExcluding string `json:"versionStartExcluding,omitempty"`
				VersionEndIncluding   string `json:"versionEndIncluding,omitempty"`
				VersionEndExcluding   string `json:"versionEndExcluding,omitempty"`
				MatchCriteriaId       string `json:"matchCriteriaId"`
			} `json:"cpeMatch"`
		}{
			{Operator: "OR", Negate: false, CpeMatch: []struct {
				Vulnerable            bool   `json:"vulnerable"`
				Criteria              string `json:"criteria"`
				VersionStartIncluding string `json:"versionStartIncluding,omitempty"`
				VersionStartExcluding string `json:"versionStartExcluding,omitempty"`
				VersionEndIncluding   string `json:"versionEndIncluding,omitempty"`
				VersionEndExcluding   string `json:"versionEndExcluding,omitempty"`
				MatchCriteriaId       string `json:"matchCriteriaId"`
			}{
				{Vulnerable: true, Criteria: "cpe:2.3:a:acme:widget:1.2.3:*:*:*:*:*:*:*", MatchCriteriaId: "id-1"},
			}},
		}},
	}
	// supply v3.1 score
	v.CVE.Metrics.CvssMetricV31 = []struct {
		Source   string `json:"source"`
		Type     string `json:"type"`
		CvssData struct {
			Version               string  `json:"version"`
			VectorString          string  `json:"vectorString"`
			AttackVector          string  `json:"attackVector"`
			AttackComplexity      string  `json:"attackComplexity"`
			PrivilegesRequired    string  `json:"privilegesRequired"`
			UserInteraction       string  `json:"userInteraction"`
			Scope                 string  `json:"scope"`
			ConfidentialityImpact string  `json:"confidentialityImpact"`
			IntegrityImpact       string  `json:"integrityImpact"`
			AvailabilityImpact    string  `json:"availabilityImpact"`
			BaseScore             float64 `json:"baseScore"`
			BaseSeverity          string  `json:"baseSeverity"`
			ExploitabilityScore   float64 `json:"exploitabilityScore"`
			ImpactScore           float64 `json:"impactScore"`
		} `json:"cvssData"`
		ExploitabilityScore float64 `json:"exploitabilityScore"`
		ImpactScore         float64 `json:"impactScore"`
	}{
		{Source: "nvd", Type: "primary", CvssData: struct {
			Version               string  `json:"version"`
			VectorString          string  `json:"vectorString"`
			AttackVector          string  `json:"attackVector"`
			AttackComplexity      string  `json:"attackComplexity"`
			PrivilegesRequired    string  `json:"privilegesRequired"`
			UserInteraction       string  `json:"userInteraction"`
			Scope                 string  `json:"scope"`
			ConfidentialityImpact string  `json:"confidentialityImpact"`
			IntegrityImpact       string  `json:"integrityImpact"`
			AvailabilityImpact    string  `json:"availabilityImpact"`
			BaseScore             float64 `json:"baseScore"`
			BaseSeverity          string  `json:"baseSeverity"`
			ExploitabilityScore   float64 `json:"exploitabilityScore"`
			ImpactScore           float64 `json:"impactScore"`
		}{Version: "3.1", VectorString: "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", BaseScore: 9.8, BaseSeverity: "CRITICAL"}, ExploitabilityScore: 3.9, ImpactScore: 5.9},
	}
	v.CVE.CISAExploitAdd = "2024-03-01"

	cve, err := db.convertToCVEInfo(v)
	if err != nil {
		t.Fatalf("convertToCVEInfo error: %v", err)
	}
	if cve.Description != "desc en" {
		t.Fatalf("want english description, got %q", cve.Description)
	}
	if cve.CVSS.Version != "3.1" || cve.CVSS.BaseScore != 9.8 || cve.Severity != "CRITICAL" {
		t.Fatalf("unexpected CVSS/severity: %#v, severity=%s", cve.CVSS, cve.Severity)
	}
	if len(cve.References) != 1 || cve.References[0].URL != "https://example.com" {
		t.Fatalf("references not mapped: %#v", cve.References)
	}
	if got := cve.PublishedDate; got.IsZero() || got.Format(time.RFC3339) == "" {
		t.Fatalf("published date not parsed: %v", got)
	}
	if len(cve.AffectedPackages) != 1 || cve.AffectedPackages[0].Name != "acme/widget" || cve.AffectedPackages[0].AffectedVersions[0] != "1.2.3" {
		t.Fatalf("affected packages not parsed: %#v", cve.AffectedPackages)
	}
	if !cve.Exploitability.HasExploit {
		t.Fatalf("expected HasExploit true")
	}
}

func TestConvertToCVEInfo_FallbackToV2(t *testing.T) {
	db := NewNVDDatabase()
	v := NVDVulnerability{}
	v.CVE.Descriptions = []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	}{{Lang: "en", Value: "x"}}
	v.CVE.Metrics.CvssMetricV2 = []struct {
		Source   string `json:"source"`
		Type     string `json:"type"`
		CvssData struct {
			Version               string  `json:"version"`
			VectorString          string  `json:"vectorString"`
			AccessVector          string  `json:"accessVector"`
			AccessComplexity      string  `json:"accessComplexity"`
			Authentication        string  `json:"authentication"`
			ConfidentialityImpact string  `json:"confidentialityImpact"`
			IntegrityImpact       string  `json:"integrityImpact"`
			AvailabilityImpact    string  `json:"availabilityImpact"`
			BaseScore             float64 `json:"baseScore"`
		} `json:"cvssData"`
		BaseSeverity            string  `json:"baseSeverity"`
		ExploitabilityScore     float64 `json:"exploitabilityScore"`
		ImpactScore             float64 `json:"impactScore"`
		AcInsufInfo             bool    `json:"acInsufInfo"`
		ObtainAllPrivilege      bool    `json:"obtainAllPrivilege"`
		ObtainUserPrivilege     bool    `json:"obtainUserPrivilege"`
		ObtainOtherPrivilege    bool    `json:"obtainOtherPrivilege"`
		UserInteractionRequired bool    `json:"userInteractionRequired"`
	}{
		{Source: "nvd", Type: "secondary", CvssData: struct {
			Version               string  `json:"version"`
			VectorString          string  `json:"vectorString"`
			AccessVector          string  `json:"accessVector"`
			AccessComplexity      string  `json:"accessComplexity"`
			Authentication        string  `json:"authentication"`
			ConfidentialityImpact string  `json:"confidentialityImpact"`
			IntegrityImpact       string  `json:"integrityImpact"`
			AvailabilityImpact    string  `json:"availabilityImpact"`
			BaseScore             float64 `json:"baseScore"`
		}{Version: "2.0", VectorString: "AV:N/AC:L/Au:N/C:P/I:P/A:P", BaseScore: 6.4}, BaseSeverity: "MEDIUM", ExploitabilityScore: 3.0, ImpactScore: 3.4},
	}
	cve, err := db.convertToCVEInfo(v)
	if err != nil {
		t.Fatalf("convertToCVEInfo error: %v", err)
	}
	if cve.CVSS.Version != "2.0" || cve.Severity != "MEDIUM" {
		t.Fatalf("expected v2 mapping, got CVSS=%#v severity=%s", cve.CVSS, cve.Severity)
	}
}
