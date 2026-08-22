// ─── Domain Classification ────────────────────────────────────────────────────

export type Domain =
  | "patent"
  | "trademark"
  | "biodiversity"
  | "regulatory";

export type Authority =
  | "parliament"
  | "ministry"
  | "tribunal"
  | "guidance";

export type Jurisdiction =
  | "india"
  | "eu"
  | "us"
  | "international";

// ─── Document Metadata ────────────────────────────────────────────────────────

export interface DocumentMetadata {
  doc_title: string;
  domain: Domain;
  jurisdiction: Jurisdiction;
  authority: Authority;
  source_url: string;
  retrieved_at: string;
}

// ─── Extraction Types ─────────────────────────────────────────────────────────

export interface ExtractedPage {
  page_num: number;
  text: string;
}

export interface ExtractedDocument {
  filename: string;
  pages: ExtractedPage[];
}

// ─── Chunk (Qdrant payload) ───────────────────────────────────────────────────

export interface Chunk {
  chunk_id: string;
  text: string;
  doc_title: string;
  domain: string;
  jurisdiction: string;
  authority: string;
  section_ref: string;
  source_url: string;
  retrieved_at: string;
  char_start: number;
  char_end: number;
}
