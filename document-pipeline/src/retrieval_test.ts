import "dotenv/config";
import { QdrantClient } from "@qdrant/js-client-rest";
import { Domain } from "./types";
import { embedQuery, EMBEDDING_MODEL } from "./embeddings";

// ─── Config ───────────────────────────────────────────────────────────────────

const QDRANT_URL      = process.env.QDRANT_URL       ?? "http://127.0.0.1:6333";
const COLLECTION_NAME = process.env.COLLECTION_NAME  ?? "ipsakti_docs";

// ─── Clients ──────────────────────────────────────────────────────────────────

const qdrant = new QdrantClient({ url: QDRANT_URL });

// ─── Types ────────────────────────────────────────────────────────────────────

export interface SearchResult {
  score:       number;
  chunk_id:    string;
  doc_title:   string;
  domain:      string;
  section_ref: string;
  text:        string;
}

// ─── Core: retrieve() ─────────────────────────────────────────────────────────

export async function retrieve(
  query:   string,
  domains: Domain[],
  topK:    number
): Promise<SearchResult[]> {
  // 1. Embed query using unified embeddings module
  const vector = await embedQuery(query);

  // 2. Build domain filter — match any of the provided domains
  const domainFilter =
    domains.length === 1
      ? { must: [{ key: "domain", match: { value: domains[0] } }] }
      : { should: domains.map((d) => ({ key: "domain", match: { value: d } })) };

  // 3. Query Qdrant
  const qRes = await qdrant.query(COLLECTION_NAME, {
    query:        vector,
    limit:        topK,
    filter:       domainFilter,
    with_payload: true,
  });

  // 4. Map to SearchResult[]
  return qRes.points.map((point) => {
    const p = (point.payload ?? {}) as Record<string, unknown>;
    return {
      score:       point.score,
      chunk_id:    (p["chunk_id"]    as string) ?? "(unknown)",
      doc_title:   (p["doc_title"]   as string) ?? "(unknown)",
      domain:      (p["domain"]      as string) ?? "(unknown)",
      section_ref: (p["section_ref"] as string) ?? "unknown",
      text:        (p["text"]        as string) ?? "",
    };
  });
}

// ─── Display helper ───────────────────────────────────────────────────────────

function printResults(results: SearchResult[]): void {
  if (results.length === 0) {
    console.log("  (no results returned — is the collection populated?)");
    return;
  }

  results.forEach((r, i) => {
    const preview = r.text.slice(0, 200).replace(/\n/g, " ");
    console.log(
      `  [${i + 1}] Score: ${r.score.toFixed(3)} | ${r.doc_title} | ` +
      `section: ${r.section_ref} | domain: ${r.domain}`
    );
    console.log(`       ${preview}…\n`);
  });
}

function header(title: string): void {
  const bar = "═".repeat(60);
  console.log(`\n${bar}`);
  console.log(`  ${title}`);
  console.log(`${bar}\n`);
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  console.log(`\n${"─".repeat(60)}`);
  console.log(`  IP-SAKTI  ·  Retrieval QA Test`);
  console.log(`  Collection : ${COLLECTION_NAME}  |  Qdrant: ${QDRANT_URL}`);
  console.log(`  Model      : ${EMBEDDING_MODEL}`);
  console.log(`${"─".repeat(60)}`);

  try {
    // ── SCENARIO 1 — Patent / Biodiversity eligibility ─────────────────────
    header("SCENARIO 1 · Patent eligibility & traditional knowledge");

    const s1Results = await retrieve(
      "proprietary Ayurvedic formulation Ashwagandha Shallaki patent eligibility traditional knowledge India",
      ["patent", "biodiversity"],
      5
    );
    printResults(s1Results);

    // ── SCENARIO 2 — Regulatory classification ─────────────────────────────
    header("SCENARIO 2 · Regulatory classification & AYUSH licensing");

    const s2Results = await retrieve(
      "Ayurvedic product classification drug food cosmetic AYUSH licensing requirement",
      ["regulatory"],
      5
    );
    printResults(s2Results);

    // ── QA advisory ────────────────────────────────────────────────────────
    console.log(
      "─".repeat(60) + "\n" +
      "  ⚠  If scores are below 0.5 or wrong domains appear,\n" +
      "     check chunking and metadata.\n" +
      "─".repeat(60) + "\n"
    );
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.error(`\n[FATAL] ${msg}`);
    process.exit(1);
  }
}

main();
