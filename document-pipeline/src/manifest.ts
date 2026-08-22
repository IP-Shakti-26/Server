import { Domain, Authority, Jurisdiction, DocumentMetadata } from "./types";

// ─── Corpus Manifest ──────────────────────────────────────────────────────────
// Maps each PDF filename to its legal metadata.
// Add new documents here before running the extract/chunk/upload pipeline.

export const CORPUS_MANIFEST: Record<string, DocumentMetadata> = {
  "patents_act_1970.pdf": {
    doc_title: "The Patents Act, 1970",
    domain: "patent" as Domain,
    authority: "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://ipindia.gov.in/patents.htm",
    retrieved_at: "2026-08-22",
  },

  "patent_rules_2003.pdf": {
    doc_title: "The Patents Rules, 2003",
    domain: "patent" as Domain,
    authority: "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://ipindia.gov.in/patents.htm",
    retrieved_at: "2026-08-22",
  },

  "biological_diversity_act_2002.pdf": {
    doc_title: "The Biological Diversity Act, 2002",
    domain: "biodiversity" as Domain,
    authority: "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://nbaindia.org/content/25/19/1/policy--legislation.html",
    retrieved_at: "2026-08-22",
  },

  "biological_diversity_rules_2004.pdf": {
    doc_title: "The Biological Diversity Rules, 2004",
    domain: "biodiversity" as Domain,
    authority: "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://nbaindia.org/content/25/19/1/policy--legislation.html",
    retrieved_at: "2026-08-22",
  },

  "nba_abs_guidelines.pdf": {
    doc_title: "NBA Guidelines on Access and Benefit Sharing (ABS)",
    domain: "biodiversity" as Domain,
    authority: "guidance" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://nbaindia.org/content/74/53/1/abs.html",
    retrieved_at: "2026-08-22",
  },

  "drugs_cosmetics_act_ayush.pdf": {
    doc_title: "The Drugs and Cosmetics Act, 1940 (AYUSH Provisions)",
    domain: "regulatory" as Domain,
    authority: "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://ayush.gov.in/about-the-systems/drugs-and-cosmetics-act",
    retrieved_at: "2026-08-22",
  },

  "new_drugs_clinical_trials_rules.pdf": {
    doc_title: "New Drugs and Clinical Trials Rules, 2019",
    domain: "regulatory" as Domain,
    authority: "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://cdsco.gov.in/opencms/opencms/en/Notifications/",
    retrieved_at: "2026-08-22",
  },

  "trade_marks_act_1999.pdf": {
    doc_title: "The Trade Marks Act, 1999",
    domain: "trademark" as Domain,
    authority: "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://ipindia.gov.in/trade-marks.htm",
    retrieved_at: "2026-08-22",
  },

  "tkdl_overview.pdf": {
    doc_title: "Traditional Knowledge Digital Library (TKDL) — Overview",
    domain: "patent" as Domain,
    authority: "guidance" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url: "https://www.tkdl.res.in/tkdl/langdefault/common/Home.asp",
    retrieved_at: "2026-08-22",
  },
};
