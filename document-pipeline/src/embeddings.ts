import "dotenv/config";
import { GoogleGenerativeAI } from "@google/generative-ai";

const OLLAMA_URL = process.env.OLLAMA_URL ?? "http://127.0.0.1:11434";
const GEMINI_API_KEY = process.env.GEMINI_API_KEY ?? "";

export type EmbeddingProvider = "ollama" | "gemini";

// Get config from .env or default to ollama
export const EMBEDDING_PROVIDER: EmbeddingProvider =
  (process.env.EMBEDDING_PROVIDER?.toLowerCase() === "gemini") ? "gemini" : "ollama";

export const EMBEDDING_MODEL =
  EMBEDDING_PROVIDER === "gemini"
    ? (process.env.GEMINI_EMBEDDING_MODEL ?? "text-embedding-004")
    : (process.env.OLLAMA_EMBEDDING_MODEL ?? "nomic-embed-text:v1.5");

// Both nomic-embed-text and text-embedding-004 output 768 dimensions by default.
export const VECTOR_SIZE = 768;

const genAI = GEMINI_API_KEY ? new GoogleGenerativeAI(GEMINI_API_KEY) : null;

async function retryWithBackoff<T>(fn: () => Promise<T>, retries = 5, delayMs = 2000): Promise<T> {
  try {
    return await fn();
  } catch (err: any) {
    const isRateLimit = err.status === 429 ||
      err.message?.includes("429") ||
      err.message?.includes("Quota exceeded") ||
      err.message?.includes("Too Many Requests");
    if (retries > 0 && isRateLimit) {
      let waitTime = delayMs;
      const match = err.message?.match(/retry in ([\d.]+)s/i);
      if (match) {
        waitTime = Math.ceil(parseFloat(match[1]) * 1000) + 1500; // safe padding
      }
      console.log(`\n  [429 Quota Exceeded] Waiting ${Math.ceil(waitTime / 1000)}s before retry...`);
      await new Promise((resolve) => setTimeout(resolve, waitTime));
      return retryWithBackoff(fn, retries - 1, delayMs * 2);
    }
    throw err;
  }
}

/**
 * Embeds a batch of texts.
 * For document indexing, set purpose to 'document'.
 * For search queries, set purpose to 'query'.
 */
export async function embedBatch(texts: string[], purpose: "document" | "query"): Promise<number[][]> {
  if (texts.length === 0) return [];

  if (EMBEDDING_PROVIDER === "gemini") {
    if (!genAI) {
      throw new Error("GEMINI_API_KEY is not set in environment, but gemini provider was selected.");
    }
    const model = genAI.getGenerativeModel({ model: EMBEDDING_MODEL });
    const taskType = purpose === "document" ? "RETRIEVAL_DOCUMENT" : "RETRIEVAL_QUERY";

    // Use batchEmbedContents API inside retry wrapper
    return retryWithBackoff(async () => {
      const result = await model.batchEmbedContents({
        requests: texts.map((t) => ({
          content: { parts: [{ text: t }] },
          taskType,
          outputDimensionality: VECTOR_SIZE,
        })),
      });

      if (!result.embeddings) {
        throw new Error("Gemini Embedding API returned empty response.");
      }
      return result.embeddings.map((e) => e.values);
    });
  } else {
    // Ollama nomic-embed-text
    const url = `${OLLAMA_URL}/api/embed`;

    // Nomic expects prefix 'search_document: ' or 'search_query: '
    const prefix = purpose === "document" ? "search_document: " : "search_query: ";
    const formattedTexts = texts.map((t) => `${prefix}${t}`);

    const response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: EMBEDDING_MODEL,
        input: formattedTexts,
      }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Ollama API error: ${response.statusText} - ${errorText}`);
    }

    const data = (await response.json()) as { embeddings: number[][] };
    return data.embeddings;
  }
}

/**
 * Helper to embed a single query text.
 */
export async function embedQuery(text: string): Promise<number[]> {
  const result = await embedBatch([text], "query");
  return result[0];
}
