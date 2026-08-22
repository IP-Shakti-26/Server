import "dotenv/config";
import { QdrantClient } from "@qdrant/js-client-rest";
import { Domain } from "./types";

// ─── Config ───────────────────────────────────────────────────────────────────

const OLLAMA_URL      = process.env.OLLAMA_URL       ?? "http://localhost:11434";
const QDRANT_URL      = process.env.QDRANT_URL       ?? "http://localhost:6333";
const COLLECTION_NAME = process.env.COLLECTION_NAME  ?? "ipsakti_docs";

const EMBEDDING_MODEL = "nomic-ipsakti"; // custom model: nomic-embed-text:v1.5 + num_ctx 4096

// ─── Clients ──────────────────────────────────────────────────────────────────

const qdrant = new QdrantClient({ url: QDRANT_URL });

// ─── Required payload fields ──────────────────────────────────────────────────

const REQUIRED_FIELDS: string[] = [
  "chunk_id", "doc_title", "domain", "jurisdiction",
  "authority", "source_url", "retrieved_at", "text",
];

// ─── Helper: embed a single query ─────────────────────────────────────────────

async function embedQuery(text: string): Promise<number[]> {
  const url = `${OLLAMA_URL}/api/embed`;
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      model: EMBEDDING_MODEL,
      input: [`search_query: ${text}`],
    }),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Ollama API error: ${response.statusText} - ${errorText}`);
  }

  const data = (await response.json()) as { embeddings: number[][] };
  return data.embeddings[0];
}

// ─── Helper: divider ─────────────────────────────────────────────────────────

function divider(title: string): void {
  console.log(`\n${"─".repeat(56)}`);
  console.log(`  ${title}`);
  console.log(`${"─".repeat(56)}`);
}

// ─── CHECK 1 — Total vector count ─────────────────────────────────────────────

async function checkTotalCount(): Promise<void> {
  divider("CHECK 1 · Total vector count");

  const info  = await qdrant.getCollection(COLLECTION_NAME);
  const count = info.points_count ?? 0;

  console.log(`  Total vectors: ${count}`);

  if (count === 0) {
    console.error("  FATAL: Collection is empty. Run embed_upload.ts first.");
    process.exit(1);
  }
}

// ─── CHECK 2 — Domain distribution ────────────────────────────────────────────

async function checkDomainDistribution(): Promise<void> {
  divider("CHECK 2 · Domain distribution");

  const domains: Domain[] = ["patent", "trademark", "biodiversity", "regulatory"];

  for (const domain of domains) {
    const res = await qdrant.count(COLLECTION_NAME, {
      filter: {
        must: [{ key: "domain", match: { value: domain } }],
      },
      exact: true,
    });

    const count = res.count;
    const icon  = count > 0 ? "✓" : "✗ MISSING";
    console.log(`  [${icon}] ${domain}: ${count} chunk${count !== 1 ? "s" : ""}`);
  }
}

// ─── CHECK 3 — Semantic spot checks ───────────────────────────────────────────

interface SpotCheck {
  query: string;
  expectedDomain: Domain;
}

const SPOT_CHECKS: SpotCheck[] = [
  {
    query:          "Section 3p patent traditional knowledge exemption",
    expectedDomain: "patent",
  },
  {
    query:          "biological diversity ABS access benefit sharing NBA",
    expectedDomain: "biodiversity",
  },
  {
    query:          "Ayurvedic drug licensing CDSCO AYUSH classification",
    expectedDomain: "regulatory",
  },
  {
    query:          "trademark distinctiveness Section 9 Ayurveda",
    expectedDomain: "trademark",
  },
];

async function checkSemanticSearch(): Promise<void> {
  divider("CHECK 3 · Semantic search spot checks");

  for (const { query, expectedDomain } of SPOT_CHECKS) {
    const vector = await embedQuery(query);

    const results = await qdrant.query(COLLECTION_NAME, {
      query:  vector,
      limit:  3,
      filter: {
        must: [{ key: "domain", match: { value: expectedDomain } }],
      },
      with_payload: true,
    });

    const domainsFound = results.points
      .map((r) => ((r.payload ?? {}) as Record<string, unknown>)["domain"] as string)
      .filter(Boolean);

    const hit  = domainsFound.includes(expectedDomain);
    const icon = hit ? "✓" : "✗ CHECK";
    const tag  = domainsFound.length ? domainsFound.join(", ") : "no results";

    console.log(`  [${icon}] "${query.slice(0, 55)}…"`);
    console.log(`         expected=${expectedDomain}  found=[${tag}]`);
  }
}

// ─── CHECK 4 — Payload completeness ───────────────────────────────────────────

async function checkPayloadCompleteness(): Promise<void> {
  divider("CHECK 4 · Payload completeness (sample of 20)");

  const res = await qdrant.scroll(COLLECTION_NAME, {
    limit:        20,
    with_payload: true,
    with_vector:  false,
  });

  const points = res.points;
  let allGood  = true;

  for (const point of points) {
    const payload = (point.payload ?? {}) as Record<string, unknown>;
    const missing = REQUIRED_FIELDS.filter((f) => !(f in payload) || payload[f] === null);

    if (missing.length > 0) {
      allGood = false;
      const id = payload["chunk_id"] ?? point.id;
      console.log(`  [✗] Point ${id} — missing fields: ${missing.join(", ")}`);
    }
  }

  if (allGood) {
    console.log(`  ✓ All ${points.length} sampled chunks have required fields`);
  }
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  console.log(`\n${"═".repeat(56)}`);
  console.log(`  IP-SAKTI  ·  Pipeline Verification`);
  console.log(`  Collection : ${COLLECTION_NAME}`);
  console.log(`  Qdrant     : ${QDRANT_URL}`);
  console.log(`${"═".repeat(56)}`);

  try {
    await checkTotalCount();
    await checkDomainDistribution();
    await checkSemanticSearch();
    await checkPayloadCompleteness();

    console.log(`\n${"═".repeat(56)}`);
    console.log(`  === Verification complete ===`);
    console.log(`${"═".repeat(56)}\n`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.error(`\n[FATAL] ${msg}`);
    process.exit(1);
  }
}

main();
