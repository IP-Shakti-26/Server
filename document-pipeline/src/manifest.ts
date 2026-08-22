import { Domain, Authority, Jurisdiction, DocumentMetadata } from "./types";

// ─── Corpus Manifest ──────────────────────────────────────────────────────────
// Maps each PDF filename (exact, case-sensitive) to its legal metadata.
// Add new documents here before running the extract/chunk/upload pipeline.

export const CORPUS_MANIFEST: Record<string, DocumentMetadata> = {

  // ── Drugs & Cosmetics ──────────────────────────────────────────────────────
  "2016DrugsandCosmeticsAct1940Rules1945.pdf": {
    doc_title:    "Drugs and Cosmetics Act, 1940 & Rules, 1945 (2016 edition)",
    domain:       "regulatory" as Domain,
    authority:    "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://cdsco.gov.in/opencms/opencms/en/Acts-Rules-Regulations/",
    retrieved_at: "2026-08-22",
  },

  // ── AYUSH / Aahar ─────────────────────────────────────────────────────────
  "Ayurveda-Aahar definition docs.pdf": {
    doc_title:    "Ayurveda Aahar — Definition & Classification Documents",
    domain:       "regulatory" as Domain,
    authority:    "guidance" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://ayush.gov.in",
    retrieved_at: "2026-08-22",
  },

  // ── Biodiversity Rules ─────────────────────────────────────────────────────
  "BIOLOGICAL DIVERSITY RULES 2004.pdf": {
    doc_title:    "The Biological Diversity Rules, 2004",
    domain:       "biodiversity" as Domain,
    authority:    "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://nbaindia.org/content/25/19/1/policy--legislation.html",
    retrieved_at: "2026-08-22",
  },

  // ── Biodiversity Act ───────────────────────────────────────────────────────
  "Biodiversity_Act_2002.pdf": {
    doc_title:    "The Biological Diversity Act, 2002",
    domain:       "biodiversity" as Domain,
    authority:    "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://nbaindia.org/content/25/19/1/policy--legislation.html",
    retrieved_at: "2026-08-22",
  },

  // ── New Drugs & Clinical Trials ────────────────────────────────────────────
  "New Drugs & Clinical Trials Rules as of 2019 amendments.pdf": {
    doc_title:    "New Drugs and Clinical Trials Rules, 2019 (with amendments)",
    domain:       "regulatory" as Domain,
    authority:    "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://cdsco.gov.in/opencms/opencms/en/Notifications/",
    retrieved_at: "2026-08-22",
  },

  // ── Patent Rules ───────────────────────────────────────────────────────────
  "The_Patent_Rules_2003.PDF": {
    doc_title:    "The Patents Rules, 2003",
    domain:       "patent" as Domain,
    authority:    "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://ipindia.gov.in/patents.htm",
    retrieved_at: "2026-08-22",
  },

  // Alias for chunk.ts which appends lowercase .pdf to the extracted stem
  "The_Patent_Rules_2003.pdf": {
    doc_title:    "The Patents Rules, 2003",
    domain:       "patent" as Domain,
    authority:    "ministry" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://ipindia.gov.in/patents.htm",
    retrieved_at: "2026-08-22",
  },

  // ── Patents Act ────────────────────────────────────────────────────────────
  "The_Patents_Act__1970___incorporating_all_amendments_till_1-08-2024.pdf": {
    doc_title:    "The Patents Act, 1970 (amended up to 01-08-2024)",
    domain:       "patent" as Domain,
    authority:    "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://ipindia.gov.in/patents.htm",
    retrieved_at: "2026-08-22",
  },

  // ── NBA ABS Guidelines ─────────────────────────────────────────────────────
  "biodiversity abs nba guideline.pdf": {
    doc_title:    "NBA Guidelines on Access and Benefit Sharing (ABS)",
    domain:       "biodiversity" as Domain,
    authority:    "guidance" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://nbaindia.org/content/74/53/1/abs.html",
    retrieved_at: "2026-08-22",
  },

  // ── Trade Marks Act ────────────────────────────────────────────────────────
  "the_trade_marks_act,_1999.pdf": {
    doc_title:    "The Trade Marks Act, 1999",
    domain:       "trademark" as Domain,
    authority:    "parliament" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://ipindia.gov.in/trade-marks.htm",
    retrieved_at: "2026-08-22",
  },

  // ── TKDL Overview (PIB) ────────────────────────────────────────────────────
  "tkdl overview PIB.pdf": {
    doc_title:    "Traditional Knowledge Digital Library (TKDL) — PIB Overview",
    domain:       "patent" as Domain,
    authority:    "guidance" as Authority,
    jurisdiction: "india" as Jurisdiction,
    source_url:   "https://pib.gov.in",
    retrieved_at: "2026-08-22",
  },

  // ── TKDL Overview (WIPO) ───────────────────────────────────────────────────
  "tkdl overview WIPO.pdf": {
    doc_title:    "Traditional Knowledge Digital Library (TKDL) — WIPO Overview",
    domain:       "patent" as Domain,
    authority:    "guidance" as Authority,
    jurisdiction: "international" as Jurisdiction,
    source_url:   "https://www.wipo.int/tkdl",
    retrieved_at: "2026-08-22",
  },

};
