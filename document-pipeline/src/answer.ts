import "dotenv/config";
import { GoogleGenerativeAI } from "@google/generative-ai";
import { retrieve, SearchResult } from "./retrieval_test";
import { Domain } from "./types";

// ─── Config ───────────────────────────────────────────────────────────────────

const GEMINI_API_KEY  = process.env.GEMINI_API_KEY ?? "";
const GENERATION_MODEL = "gemini-1.5-flash";

if (!GEMINI_API_KEY) {
  console.error("[ERROR] GEMINI_API_KEY is not set in .env");
  process.exit(1);
}

const genAI = new GoogleGenerativeAI(GEMINI_API_KEY);

// ─── Language Types ───────────────────────────────────────────────────────────

export type ResponseLanguage = "hindi" | "english" | "hinglish" | "auto";

// ─── System Prompt ────────────────────────────────────────────────────────────

function buildSystemPrompt(language: ResponseLanguage): string {
  const langInstructions: Record<ResponseLanguage, string> = {
    english: `
You are IP-SAKTI, an expert legal assistant specialising in Indian Intellectual Property (IP) law,
biodiversity regulations, AYUSH licensing, and traditional knowledge protection.

LANGUAGE RULE: Always respond in clear, formal English.
`,
    hindi: `
You are IP-SAKTI, an expert legal assistant specialising in Indian Intellectual Property (IP) law,
biodiversity regulations, AYUSH licensing, and traditional knowledge protection.

LANGUAGE RULE: हमेशा हिंदी में उत्तर दें। आपका उत्तर स्पष्ट, सरल और औपचारिक हिंदी में होना चाहिए।
कानूनी शब्दों के लिए आप अंग्रेज़ी शब्द इस्तेमाल कर सकते हैं जैसे "Patent", "Trademark" आदि,
लेकिन बाकी सभी व्याख्या हिंदी में दें।
`,
    hinglish: `
You are IP-SAKTI, an expert legal assistant specialising in Indian Intellectual Property (IP) law,
biodiversity regulations, AYUSH licensing, and traditional knowledge protection.

LANGUAGE RULE: Hinglish mein jawab do — Hindi aur English ka mix. Jo bhi technical/legal
terms hain (jaise "Patent", "Trademark", "prior art") unhe English mein hi rakho, baaki
sab Hinglish mein explain karo. Casual, friendly tone rakho lekin information accurate ho.
`,
    auto: `
You are IP-SAKTI, an expert legal assistant specialising in Indian Intellectual Property (IP) law,
biodiversity regulations, AYUSH licensing, and traditional knowledge protection.

LANGUAGE RULE: Detect the language of the user's question and reply in the SAME language:
- If the question is in Hindi → reply in Hindi (same rules as Hindi mode).
- If the question is in Hinglish (mixed Hindi-English) → reply in Hinglish.
- If the question is in English → reply in English.
`,
  };

  return `
${langInstructions[language].trim()}

BEHAVIOUR RULES:
1. Answer ONLY using the provided context chunks. Do not hallucinate.
2. If the context does not contain enough information, clearly say so in the chosen language.
3. Always cite the source document and section when referencing a legal provision.
4. Be concise but complete. Avoid unnecessary padding.
5. If the question is ambiguous, mention the ambiguity before answering.
`.trim();
}

// ─── Context Builder ──────────────────────────────────────────────────────────

function buildContext(results: SearchResult[]): string {
  if (results.length === 0) return "(No relevant documents found.)";

  return results
    .map(
      (r, i) =>
        `[Source ${i + 1}] ${r.doc_title} | Section: ${r.section_ref} | Domain: ${r.domain}\n` +
        r.text
    )
    .join("\n\n---\n\n");
}

// ─── Core: generateAnswer() ───────────────────────────────────────────────────

export interface AnswerResult {
  question:   string;
  language:   ResponseLanguage;
  answer:     string;
  sources:    { doc_title: string; section_ref: string; score: number }[];
}

export async function generateAnswer(
  question: string,
  domains:  Domain[],
  language: ResponseLanguage = "auto",
  topK:     number           = 5
): Promise<AnswerResult> {
  // 1. Retrieve relevant chunks from Qdrant
  const results = await retrieve(question, domains, topK);

  // 2. Build the prompt
  const context = buildContext(results);
  const userPrompt =
    `CONTEXT:\n${context}\n\n` +
    `QUESTION: ${question}`;

  // 3. Call Gemini for generation
  const model = genAI.getGenerativeModel({
    model:          GENERATION_MODEL,
    systemInstruction: buildSystemPrompt(language),
  });

  const chat   = model.startChat();
  const result = await chat.sendMessage(userPrompt);
  const answer = result.response.text();

  // 4. Return structured result
  return {
    question,
    language,
    answer,
    sources: results.map((r) => ({
      doc_title:   r.doc_title,
      section_ref: r.section_ref,
      score:       r.score,
    })),
  };
}

// ─── CLI Demo ─────────────────────────────────────────────────────────────────
// Run directly:  npx ts-node src/answer.ts
// Accepts optional args:  --lang hindi|english|hinglish|auto  --query "your question"

async function main(): Promise<void> {
  const args    = process.argv.slice(2);
  const langIdx = args.indexOf("--lang");
  const qIdx    = args.indexOf("--query");

  const language = (langIdx !== -1 ? args[langIdx + 1] : "auto") as ResponseLanguage;
  const question =
    qIdx !== -1
      ? args.slice(qIdx + 1).join(" ")
      : "What are the requirements to patent an Ayurvedic formulation in India?";

  const validLangs: ResponseLanguage[] = ["hindi", "english", "hinglish", "auto"];
  if (!validLangs.includes(language)) {
    console.error(`[ERROR] --lang must be one of: ${validLangs.join(", ")}`);
    process.exit(1);
  }

  console.log(`\n${"─".repeat(60)}`);
  console.log(`  IP-SAKTI  ·  Multilingual Q&A`);
  console.log(`  Language : ${language}  |  Model: ${GENERATION_MODEL}`);
  console.log(`  Question : ${question}`);
  console.log(`${"─".repeat(60)}\n`);

  try {
    const result = await generateAnswer(
      question,
      ["patent", "biodiversity", "regulatory", "trademark"],
      language
    );

    console.log("ANSWER:\n");
    console.log(result.answer);

    console.log(`\n${"─".repeat(60)}`);
    console.log("SOURCES USED:");
    result.sources.forEach((s, i) => {
      console.log(`  [${i + 1}] ${s.doc_title} | ${s.section_ref} (score: ${s.score.toFixed(3)})`);
    });
    console.log(`${"─".repeat(60)}\n`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.error(`\n[FATAL] ${msg}`);
    process.exit(1);
  }
}

main();
