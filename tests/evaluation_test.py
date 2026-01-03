import sys
import os
import csv
import time
import statistics
import re
from collections import defaultdict
from sentence_transformers import SentenceTransformer

# Add clients/python to path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "clients", "python"))

try:
    from waddle_client import WaddleClient
except ImportError:
    print(
        "Error: Could not import WaddleClient. Make sure you are running this from the tests directory or have the python client generated."
    )
    sys.exit(1)

MODEL_NAME = "all-MiniLM-L6-v2"
COLLECTION_NAME = "eval_db"
DATA_DIR = os.path.join(os.path.dirname(__file__), "archive")

# Increase CSV field size limit
csv.field_size_limit(10 * 1024 * 1024)  # 10MB


def load_data():
    documents = []
    with open(os.path.join(DATA_DIR, "documents.csv"), "r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            documents.append(row)

    qa_single = []
    with open(
        os.path.join(DATA_DIR, "single_passage_answer_questions.csv"),
        "r",
        encoding="utf-8",
    ) as f:
        reader = csv.DictReader(f)
        for row in reader:
            qa_single.append(row)

    qa_multi = []
    with open(
        os.path.join(DATA_DIR, "multi_passage_answer_questions.csv"),
        "r",
        encoding="utf-8",
    ) as f:
        reader = csv.DictReader(f)
        for row in reader:
            qa_multi.append(row)

    qa_no_answer = []
    with open(
        os.path.join(DATA_DIR, "no_answer_questions.csv"), "r", encoding="utf-8"
    ) as f:
        reader = csv.DictReader(f)
        for row in reader:
            qa_no_answer.append(row)

    return documents, qa_single, qa_multi, qa_no_answer


def chunk_text(text, min_size=100, max_size=1000):
    # Normalize
    text = text.replace("\r\n", "\n")

    # Split by paragraphs
    paragraphs = text.split("\n\n")
    chunks = []
    current_chunk = ""

    for para in paragraphs:
        para = para.strip()
        if not para:
            continue

        if len(current_chunk) + len(para) < max_size:
            if current_chunk:
                current_chunk += "\n\n" + para
            else:
                current_chunk = para
        else:
            # Current chunk is full, verify min size
            if len(current_chunk) >= min_size:
                chunks.append(current_chunk)
                current_chunk = para
            else:
                # Too small even after adding? append anyway if we have no choice,
                # or better: merge regardless of max_size if it was empty,
                # but here we are in "else" meaning current_chunk + para > max_size.
                # If current_chunk is small but adding para makes it too big,
                # we probably have to split para or just accepting a slightly larger chunk.
                # Let's just append current and start new.
                if current_chunk:
                    chunks.append(current_chunk)
                current_chunk = para

    if current_chunk:
        chunks.append(current_chunk)

    return chunks


def evaluate_scenario(collection, model, name, questions, k=5):
    print(f"\n--- Running Scenario: {name} ---")
    latencies = []
    distances = []
    hits = 0
    total = len(questions)

    for i, item in enumerate(questions):
        q_text = item["question"]
        target_doc_idx = item["document_index"]

        start_time = time.time()
        q_vec = model.encode(q_text).tolist()
        try:
            results = collection.search(q_vec, top_k=k)
        except Exception as e:
            print(f"Search error for query '{q_text}': {e}")
            continue
        duration = (time.time() - start_time) * 1000  # ms
        latencies.append(duration)

        if not results:
            print(f"  [WARN] No results for '{q_text}'")
            continue

        top_dist = results[0].distance
        distances.append(top_dist)

        # Check Hit (if any result comes from the target document)
        # We stored key as "doc_{index}"
        found = False
        for res in results:
            if res.key == f"doc_{target_doc_idx}":
                found = True
                break

        if found:
            hits += 1

    avg_lat = statistics.mean(latencies) if latencies else 0
    p95_lat = (
        statistics.quantiles(latencies, n=20)[18]
        if len(latencies) >= 20
        else max(latencies) if latencies else 0
    )
    avg_dist = statistics.mean(distances) if distances else 0
    recall = hits / total if total > 0 else 0

    print(f"  Results for {name}:")
    print(f"  Total Queries: {total}")
    print(f"  Recall@{k}: {recall:.2%}")
    print(f"  Avg Latency: {avg_lat:.2f} ms")
    print(f"  P95 Latency: {p95_lat:.2f} ms")
    print(f"  Avg Top-1 Distance: {avg_dist:.4f}")

    return {
        "Scenario": name,
        "AvgLatencyMs": avg_lat,
        "P95LatencyMs": p95_lat,
        "RecallAt5": recall,
        "AvgDist": avg_dist,
    }


def main():
    print("Loading data...")
    docs, qa_single, qa_multi, qa_no_answer = load_data()
    print(f"Loaded {len(docs)} documents.")
    print(f"Loaded {len(qa_single)} single-passage questions.")
    print(f"Loaded {len(qa_multi)} multi-passage questions.")
    print(f"Loaded {len(qa_no_answer)} no-answer questions.")

    print("\nInitializing WaddleDB Client...")
    try:
        client = WaddleClient(host="localhost", port=6969)
    except Exception as e:
        print(f"Failed to connect: {e}")
        sys.exit(1)

    # Reset Collection
    try:
        client.delete_collection(COLLECTION_NAME)
    except:
        pass

    collection = client.create_collection(COLLECTION_NAME, 384)

    print("\nLoading Model...")
    model = SentenceTransformer(MODEL_NAME)

    print("\n--- Starting Ingestion ---")
    start_ingest = time.time()
    total_chunks = 0

    for row in docs:
        idx = row["index"]
        text = row["text"]
        key_name = f"doc_{idx}"

        chunks = chunk_text(text)
        total_chunks += len(chunks)

        for chunk in chunks:
            vec = model.encode(chunk).tolist()
            # Ingest
            collection.append_block(
                key=key_name, primary=chunk, vector=vec, keywords=[key_name]
            )

    ingest_duration = time.time() - start_ingest
    print(f"Ingestion complete in {ingest_duration:.2f}s.")
    print(f"Total Chunks: {total_chunks}")
    print(f"Chunks/Sec: {total_chunks/ingest_duration:.2f}")

    results = []

    # 1. Single Passage
    res = evaluate_scenario(collection, model, "Single Passage", qa_single)
    results.append(res)

    # 2. Multi Passage
    res = evaluate_scenario(collection, model, "Multi Passage", qa_multi)
    results.append(res)

    # 3. No Answer
    # For No Answer, "Recall" means "did we retrieve the relevant doc?"
    # Even if there is no answer, meaningful retrieval should still point to the relevant topic/doc.
    # The CSV has document_index, so we assume that's the "relevant" doc.
    res = evaluate_scenario(collection, model, "No Answer", qa_no_answer)
    results.append(res)

    print("\n--- Final Summary ---")
    print(
        f"{'Scenario':<15} | {'Avg Lat (ms)':<12} | {'P95 Lat (ms)':<12} | {'Recall@5':<10} | {'Avg Dist':<10}"
    )
    print("-" * 70)
    for r in results:
        print(
            f"{r['Scenario']:<15} | {r['AvgLatencyMs']:<12.2f} | {r['P95LatencyMs']:<12.2f} | {r['RecallAt5']:<10.2%} | {r['AvgDist']:<10.4f}"
        )

    client.close()


if __name__ == "__main__":
    main()
