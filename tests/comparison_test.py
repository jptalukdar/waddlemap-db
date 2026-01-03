import sys
import os
import csv
import time
import statistics
import shutil
import chromadb
from sentence_transformers import SentenceTransformer

# Add clients/python to path
sys.path.append(os.path.join(os.path.dirname(__file__), '..', 'clients', 'python'))

try:
    from waddle_client import WaddleClient
except ImportError:
    print("Error: Could not import WaddleClient.")
    sys.exit(1)

# Increase CSV field size limit
csv.field_size_limit(10 * 1024 * 1024) # 10MB

MODEL_NAME = 'all-MiniLM-L6-v2'
WADDLE_COL = "comp_waddle"
CHROMA_COL = "comp_chroma"
DATA_DIR = os.path.join(os.path.dirname(__file__), 'archive')
CHROMA_PATH = os.path.join(os.path.dirname(__file__), 'chroma_data')

def load_data():
    documents = []
    with open(os.path.join(DATA_DIR, 'documents.csv'), 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            documents.append(row)
            
    qa_single = []
    with open(os.path.join(DATA_DIR, 'single_passage_answer_questions.csv'), 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            qa_single.append(row)

    qa_multi = []
    with open(os.path.join(DATA_DIR, 'multi_passage_answer_questions.csv'), 'r', encoding='utf-8') as f:
        try:
            reader = csv.DictReader(f)
            for row in reader:
                qa_multi.append(row)
        except Exception as e:
            print(f"Warning: Error reading multi_passage csv: {e}")
            
    qa_no_answer = []
    with open(os.path.join(DATA_DIR, 'no_answer_questions.csv'), 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            qa_no_answer.append(row)

    return documents, qa_single, qa_multi, qa_no_answer

def chunk_text(text, min_size=100, max_size=1000):
    text = text.replace('\r\n', '\n')
    paragraphs = text.split('\n\n')
    chunks = []
    current_chunk = ""
    
    for para in paragraphs:
        para = para.strip()
        if not para:
            continue
        if len(current_chunk) + len(para) < max_size:
            current_chunk += "\n\n" + para if current_chunk else para
        else:
            if len(current_chunk) >= min_size:
                chunks.append(current_chunk)
                current_chunk = para
            else:
                if current_chunk:
                    chunks.append(current_chunk)
                current_chunk = para
    if current_chunk:
        chunks.append(current_chunk)
    return chunks

def main():
    print("--- WaddleDB vs ChromaDB Benchmark ---")
    
    # Clean ChromaDB path
    if os.path.exists(CHROMA_PATH):
        try:
            shutil.rmtree(CHROMA_PATH)
            print("Cleaned existing ChromaDB data.")
        except Exception as e:
            print(f"Warning: Could not clean ChromaDB data: {e}")

    print("Loading Data...")
    docs, qa_single, qa_multi, qa_no_answer = load_data()
    
    print("Loading Model...")
    model = SentenceTransformer(MODEL_NAME)
    
    print("Preparing Chunks & Embeddings (for fair comparison)...")
    # We pre-compute everything so ingestion measurement is purely DB write time
    # list of (doc_id, chunk_index, text, embedding)
    prepared_data = []
    
    start_prep = time.time()
    for row in docs:
        idx = row['index']
        chunks = chunk_text(row['text'])
        for c_i, chunk in enumerate(chunks):
            vec = model.encode(chunk).tolist()
            prepared_data.append({
                'doc_id': idx,
                'chunk_idx': c_i,
                'text': chunk,
                'vector': vec
            })
    print(f"Prepared {len(prepared_data)} chunks in {time.time()-start_prep:.2f}s.")

    # --- WaddleDB Setup ---
    print("\n[WaddleDB] Connecting...")
    try:
        w_client = WaddleClient(host='localhost', port=6969)
        try: w_client.delete_collection(WADDLE_COL)
        except: pass
        w_client.create_collection(WADDLE_COL, 384)
    except Exception as e:
        print(f"WaddleDB Error: {e}")
        sys.exit(1)

    # --- ChromaDB Setup ---
    print("\n[ChromaDB] Connecting...")
    c_client = chromadb.PersistentClient(path=CHROMA_PATH)
    c_collection = c_client.create_collection(name=CHROMA_COL, metadata={"hnsw:space": "cosine"}) # Use cosine to match

    # --- Ingestion Benchmark ---
    
    print("\n[Ingestion Benchmark]")
    
    # WaddleDB Ingestion
    print("  Ingesting into WaddleDB...")
    start_w = time.time()
    for item in prepared_data:
        key = f"doc_{item['doc_id']}"
        w_client.append_block(
            collection=WADDLE_COL,
            key=key,
            primary=item['text'],
            vector=item['vector'],
            keywords=[key]
        )
    w_ingest_time = time.time() - start_w
    print(f"  WaddleDB Ingestion: {w_ingest_time:.4f}s ({len(prepared_data)/w_ingest_time:.1f} chunks/s)")
    
    # ChromaDB Ingestion
    print("  Ingesting into ChromaDB...")
    start_c = time.time()
    # Batch add for Chroma is efficient, but let's do item-by-item to be comparable to the loop above?
    # Or batch it because that's idiomatic Chroma?
    # WaddleDB client doesn't support batch append yet?
    # To be fair to the "use case", we usually batch for Chroma.
    # But if we did one-by-one for Waddle, we should try to match or use best practice for each.
    # I will use batching for Chroma (max batch size e.g. 100 or all) to show its "best" performance,
    # and note if Waddle is slow it might be due to lack of batching.
    # Actually, let's use a batch size of 100 for Chroma.
    
    batch_size = 100
    ids = []
    embeddings = []
    documents = []
    metadatas = []
    
    for i, item in enumerate(prepared_data):
        unique_id = f"doc_{item['doc_id']}_chunk_{item['chunk_idx']}"
        ids.append(unique_id)
        embeddings.append(item['vector'])
        documents.append(item['text'])
        metadatas.append({"doc_id": item['doc_id']})
        
        if len(ids) >= batch_size:
            c_collection.add(ids=ids, embeddings=embeddings, documents=documents, metadatas=metadatas)
            ids, embeddings, documents, metadatas = [], [], [], []
            
    if ids:
        c_collection.add(ids=ids, embeddings=embeddings, documents=documents, metadatas=metadatas)
        
    c_ingest_time = time.time() - start_c
    print(f"  ChromaDB Ingestion: {c_ingest_time:.4f}s ({len(prepared_data)/c_ingest_time:.1f} chunks/s)")

    # --- Search Benchmark ---
    
    scenarios = [
        ("Single Passage", qa_single),
        ("Multi Passage", qa_multi),
        ("No Answer", qa_no_answer),
    ]
    
    results = []
    
    for name, questions in scenarios:
        print(f"\nRunning Scenario: {name}")
        w_lats = []
        c_lats = []
        w_hits = 0
        c_hits = 0
        total = len(questions)
        
        for q_item in questions:
            q_text = q_item['question']
            doc_idx= q_item.get('document_index')
            target_key_w = f"doc_{doc_idx}"       # Waddle key
            target_meta_c = str(doc_idx)         # Chroma metadata 'doc_id'
            
            q_vec = model.encode(q_text).tolist()
            
            # Waddle Search
            sw = time.time()
            w_res = w_client.search(WADDLE_COL, q_vec, top_k=5)
            w_lats.append((time.time() - sw) * 1000)
            
            # Chroma Search
            sc = time.time()
            c_res = c_collection.query(query_embeddings=[q_vec], n_results=5)
            c_lats.append((time.time() - sc) * 1000)
            
            # Check Waddle Hit
            found_w = False
            if w_res:
                for r in w_res:
                    if r.key == target_key_w:
                        found_w = True
                        break
            if found_w: w_hits += 1
            
            # Check Chroma Hit
            # c_res['metadatas'][0] is list of dicts
            found_c = False
            if c_res and c_res['metadatas']:
                for m in c_res['metadatas'][0]:
                    if str(m.get('doc_id')) == str(doc_idx):
                        found_c = True
                        break
            if found_c: c_hits += 1

        w_avg = statistics.mean(w_lats)
        c_avg = statistics.mean(c_lats)
        w_rec = w_hits/total
        c_rec = c_hits/total
        
        results.append({
            "Scenario": name,
            "W_Lat": w_avg,
            "C_Lat": c_avg,
            "W_Rec": w_rec,
            "C_Rec": c_rec
        })

    print("\n--- Comparative Results ---")
    print(f"{'Scenario':<15} | {'Waddle Lat':<10} | {'Chroma Lat':<10} | {'Waddle Rec':<10} | {'Chroma Rec':<10}")
    print("-" * 65)
    for r in results:
        print(f"{r['Scenario']:<15} | {r['W_Lat']:.2f} ms    | {r['C_Lat']:.2f} ms    | {r['W_Rec']:.1%}     | {r['C_Rec']:.1%}")
        
    print(f"\nIngestion Speed:")
    print(f"  WaddleDB: {w_ingest_time:.2f}s")
    print(f"  ChromaDB: {c_ingest_time:.2f}s")

    w_client.close()

if __name__ == "__main__":
    main()
