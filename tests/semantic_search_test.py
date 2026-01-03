import sys
import os
import re
from sentence_transformers import SentenceTransformer
import json

# Add clients/python to path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "clients", "python"))

try:
    from waddle_client import WaddleClient
except ImportError:
    print(
        "Error: Could not import WaddleClient. Make sure you are running this from the tests directory or have the python client generated."
    )
    sys.exit(1)


def parse_markdown(file_path):
    with open(file_path, "r", encoding="utf-8") as f:
        content = f.read()

    # Split by headers (## or ###)
    # Using a regex lookahead to keep the delimiter
    chunks = []

    # Normalize newlines
    content = content.replace("\r\n", "\n")

    # Split content based on headers, keeping the headers
    # The pattern looks for lines starting with ## or ###
    parts = re.split(r"(^#{2,3} .*)", content, flags=re.MULTILINE)

    current_chunk = []

    # The first part might be pre-header content (like title), keep it if not empty
    if parts[0].strip():
        chunks.append({"title": "Introduction", "content": parts[0].strip()})

    for i in range(1, len(parts), 2):
        header = parts[i].strip()
        body = parts[i + 1].strip() if i + 1 < len(parts) else ""

        # Create a keyword from header
        # remove #, strip, lower, replace space with _
        clean_header = header.lstrip("#").strip()
        keyword = re.sub(r"[^a-zA-Z0-9]", "_", clean_header.lower())

        chunks.append(
            {
                "title": clean_header,
                "keyword": keyword,
                "content": header + "\n\n" + body,
            }
        )

    return chunks


def parse_queries(file_path):
    with open(file_path, "r", encoding="utf-8") as f:
        lines = f.readlines()

    queries = []
    for line in lines:
        if line.strip().startswith('Query: "'):
            # Extract text between quotes
            match = re.search(r'Query: "(.*)"', line)
            if match:
                queries.append(match.group(1))
    return queries


def main():
    print("Initializing WaddleDB Client...")
    try:
        client = WaddleClient(host="localhost", port=6969)
    except Exception as e:
        print(f"Failed to connect to WaddleDB: {e}")
        print("Please ensure the server is running on localhost:6969")
        sys.exit(1)

    collection_name = "omega_db"

    # Clean up previous run
    try:
        client.delete_collection(collection_name)
        print(f"Deleted existing collection '{collection_name}'")
    except:
        pass  # Collection might not exist

    print(f"Creating collection '{collection_name}' with dimensions=384...")
    try:
        collection = client.create_collection(collection_name, 384)
    except Exception as e:
        print(f"Failed to create collection: {e}")
        sys.exit(1)

    print("Loading SentenceTransformer model...")
    model = SentenceTransformer("all-MiniLM-L6-v2")

    print("Parsing document...")
    data_path = os.path.join(os.path.dirname(__file__), "data", "small_data.md")
    chunks = parse_markdown(data_path)
    print(f"Found {len(chunks)} chunks.")

    print("Ingesting chunks...")
    doc_name = "small_data.md"

    for i, chunk in enumerate(chunks):
        embedding = model.encode(chunk["content"]).tolist()
        keywords = []
        if "keyword" in chunk:
            # Prefix with index to match user request requirement (e.g. 2_0_system_architecture)
            # We don't have explicit section numbers in the simplified parser,
            # but we can simulate or just use the header slug.
            # The prompt asked for "identify section number", but the sample file
            # only has headers. Let's make an approximation or just use the slug.
            # prompt: "add header with section number as a keyword (eg. 2_0_system_architecture"
            # We'll just use the slug for now to be safe.
            keywords.append(chunk["keyword"])

        print(f"  Ingesting chunk {i}: {chunk.get('title', 'Intro')}")

        # Append block.
        # Note: append_block(collection, key, primary, vector, keywords)
        # Using the SAME key 'small_data.md' for all blocks implies we are appending
        # multiple blocks to the same key?
        # The prompt says: "The key is the document name."
        # WaddleDB append_block semantic: usually key maps to a list of blocks?
        # Let's check waddle_client.py again if I can...
        # Or I just assume append_block does what it says.

        try:
            collection.append_block(
                key=doc_name,
                primary=chunk["content"],  # Primary is string in proto
                vector=embedding,
                keywords=keywords,
            )
        except Exception as e:
            print(f"Error ingesting chunk {i}: {e}")

    print("\n--- Running Search Tests ---\n")

    query_path = os.path.join(os.path.dirname(__file__), "data", "search_queries.md")
    queries = parse_queries(query_path)
    query_results = {}
    for q in queries:
        print(f'Query: "{q}"')
        q_vec = model.encode(q).tolist()

        try:
            results = collection.search(q_vec, top_k=1)
            if not results:
                print("  No results found.")
            else:
                for res in results:
                    # Result has: key, score, data (BlockData)
                    # BlockData has: primary (string), vector, keywords
                    # Let's print the preview of primary
                    # print(res)
                    query_results[q] = res.block.primary
                    content_preview = res.block.primary[:100].replace("\n", " ")
                    print(
                        f"  Found: {res.distance:.4f} | {res.key} | {content_preview}..."
                    )
        except Exception as e:
            print(f"  Search error: {e}")
        print("")

    client.close()

    with open("results.json", "w") as f:
        json.dump(query_results, f, indent=4)


if __name__ == "__main__":
    main()
