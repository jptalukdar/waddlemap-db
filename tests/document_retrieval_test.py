"""
Document Retrieval Test Suite
=============================

This test file validates document-level retrieval capabilities for WaddleDB.

Test Goals:
1. Given multiple documents, identify top-k documents instead of top-k chunks
2. Given all documents, identify top-k documents tagged with keyword x
3. Given a structured document (e.g., legal), chunks should reflect section references
4. Given a document, ask questions based on that single document only

"""

import sys
import os
import re
from typing import List, Dict, Any

# Add clients/python to path
sys.path.append(os.path.join(os.path.dirname(__file__), "..", "clients", "python"))

try:
    from waddle_client import WaddleClient, Collection
except ImportError:
    print(
        "Error: Could not import WaddleClient. Make sure you are running this from the tests directory."
    )
    sys.exit(1)


class bcolors:
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'


class MockEmbedder:
    """
    Simple mock embedder for testing.
    Uses deterministic hashing to create consistent 4-dimensional vectors.
    In production, replace with SentenceTransformer or similar.
    """
    
    def __init__(self, dim: int = 4):
        self.dim = dim
    
    def encode(self, text: str) -> List[float]:
        """Generate a deterministic vector from text content."""
        import hashlib
        h = hashlib.md5(text.encode()).hexdigest()
        # Convert first 4 hex pairs to floats in range [0, 1]
        vector = []
        for i in range(self.dim):
            byte_val = int(h[i*2:i*2+2], 16)
            vector.append(byte_val / 255.0)
        return vector


class DocumentRetrievalTestContext:
    """Test context for document retrieval tests."""
    
    def __init__(self, host: str = "localhost", port: int = 6969):
        self.client = WaddleClient(host, port)
        self.embedder = MockEmbedder(dim=4)
        self.collections_created: List[str] = []
    
    def cleanup(self):
        print(f"\n{bcolors.WARNING}Cleaning up test collections...{bcolors.ENDC}")
        for name in self.collections_created:
            try:
                self.client.delete_collection(name)
                print(f"  Deleted {name}")
            except Exception as e:
                print(f"  Failed to delete {name}: {e}")
        self.client.close()
    
    def create_collection(self, name: str, dims: int = 4, metric: str = "l2") -> Collection:
        """Create a test collection, cleaning up any existing one with the same name."""
        try:
            self.client.delete_collection(name)
        except:
            pass
        
        col = self.client.create_collection(name, dims, metric)
        if name not in self.collections_created:
            self.collections_created.append(name)
        return col


class BaseDocumentTest:
    """Base class for document retrieval tests."""
    
    def __init__(self, ctx: DocumentRetrievalTestContext):
        self.ctx = ctx
    
    def log(self, msg: str):
        print(f"    {msg}")
    
    def assert_true(self, condition: bool, msg: str):
        if not condition:
            raise AssertionError(msg)
    
    def assert_equal(self, a: Any, b: Any, msg: str):
        if a != b:
            raise AssertionError(f"{msg}: {a} != {b}")
    
    def assert_in(self, item: Any, container: Any, msg: str):
        if item not in container:
            raise AssertionError(f"{msg}: {item} not in {container}")
    
    def run(self):
        raise NotImplementedError


# =============================================================================
# Test 1: Top-K Documents Instead of Top-K Chunks
# =============================================================================

class TopKDocumentsTest(BaseDocumentTest):
    """
    Goal: Given multiple documents, identify top-k documents instead of top-k chunks.
    
    Test Scenario:
    - Ingest multiple documents (each with multiple chunks)
    - When searching, aggregate results by document key and return top-k unique documents
    - Verify that we get k unique document keys, not k chunks from potentially the same document
    """
    
    def run(self):
        print(f"{bcolors.HEADER}[Test 1] Top-K Documents Retrieval{bcolors.ENDC}")
        
        col = self.ctx.create_collection("test_topk_docs", dims=4, metric="l2")
        
        # Create 5 documents with 3 chunks each
        documents = {
            "doc_legal_contract.pdf": [
                "This agreement is entered into between Party A and Party B.",
                "The terms of payment shall be net 30 days from invoice date.",
                "Termination clause: Either party may terminate with 30 days notice.",
            ],
            "doc_tech_manual.pdf": [
                "Installation guide for software version 2.5 including system requirements.",
                "Troubleshooting section covering common errors and solutions.",
                "API reference documentation with endpoint specifications.",
            ],
            "doc_financial_report.pdf": [
                "Q3 2024 earnings summary showing 15% revenue growth.",
                "Balance sheet analysis and asset allocation details.",
                "Future projections and market expansion strategies.",
            ],
            "doc_hr_policy.pdf": [
                "Employee onboarding procedures and orientation guidelines.",
                "Leave policy including vacation, sick, and parental leave.",
                "Performance review process and evaluation criteria.",
            ],
            "doc_project_plan.pdf": [
                "Project timeline with key milestones and deliverables.",
                "Resource allocation and team assignment matrix.",
                "Risk assessment and mitigation strategies.",
            ],
        }
        
        # Ingest all documents and chunks
        self.log("Ingesting 5 documents with 3 chunks each...")
        for doc_key, chunks in documents.items():
            for i, chunk_content in enumerate(chunks):
                vector = self.ctx.embedder.encode(chunk_content)
                col.append_block(
                    key=doc_key,
                    primary=chunk_content,
                    vector=vector,
                    keywords=[f"chunk_{i}", doc_key.split("_")[1]]  # e.g., "legal", "tech"
                )
        
        # Search query that might match multiple chunks
        query = "payment terms and invoice processing"
        query_vector = self.ctx.embedder.encode(query)
        
        # Scenario 1: Regular search (returns chunks, might have duplicates from same doc)
        self.log("Performing standard search (top_k=5 chunks)...")
        results = col.search(query_vector, top_k=5)
        
        # Count unique documents in results
        chunk_doc_keys = [res.key for res in results]
        unique_docs = list(set(chunk_doc_keys))
        self.log(f"  Found {len(results)} chunks from {len(unique_docs)} unique documents")
        
        # Scenario 2: Aggregate by document (client-side for now)
        # In real implementation, this should be a server-side feature
        self.log("Aggregating results by document...")
        doc_scores: Dict[str, float] = {}
        for res in results:
            if res.key not in doc_scores or res.distance < doc_scores[res.key]:
                doc_scores[res.key] = res.distance
        
        top_k_docs = sorted(doc_scores.items(), key=lambda x: x[1])[:3]
        self.log(f"  Top 3 documents: {[d[0] for d in top_k_docs]}")
        
        # Verify we can select k unique documents
        self.assert_true(
            len(top_k_docs) == 3,
            "Should return exactly 3 unique documents"
        )
        
        # Verify documents are distinct
        doc_names = [d[0] for d in top_k_docs]
        self.assert_equal(
            len(doc_names), 
            len(set(doc_names)), 
            "All returned documents should be unique"
        )
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")


# =============================================================================
# Test 2: Top-K Documents with Keyword Filter
# =============================================================================

class TopKDocumentsWithKeywordTest(BaseDocumentTest):
    """
    Goal: Given all documents, identify top-k documents tagged with keyword x.
    
    Test Scenario:
    - Ingest documents with various keyword tags
    - Filter search to only include documents with specific keyword
    - Return top-k documents matching both semantic similarity AND keyword filter
    """
    
    def run(self):
        print(f"{bcolors.HEADER}[Test 2] Top-K Documents with Keyword Filter{bcolors.ENDC}")
        
        col = self.ctx.create_collection("test_topk_keyword", dims=4, metric="l2")
        
        # Documents with category tags
        documents = [
            {
                "key": "contract_alpha.pdf",
                "chunks": [
                    "Service agreement between Alpha Corp and Beta LLC.",
                    "Payment schedule: Monthly installments over 12 months.",
                ],
                "tags": ["legal", "contract", "confidential"]
            },
            {
                "key": "contract_beta.pdf",
                "chunks": [
                    "Non-disclosure agreement for project Beta.",
                    "Breach penalties and enforcement clauses.",
                ],
                "tags": ["legal", "nda", "confidential"]
            },
            {
                "key": "report_q3.pdf",
                "chunks": [
                    "Quarterly financial summary with revenue metrics.",
                    "Expense breakdown by department.",
                ],
                "tags": ["financial", "report", "quarterly"]
            },
            {
                "key": "report_annual.pdf",
                "chunks": [
                    "Annual performance review for fiscal year 2024.",
                    "Key performance indicators and targets.",
                ],
                "tags": ["financial", "report", "annual"]
            },
            {
                "key": "manual_product.pdf",
                "chunks": [
                    "Product installation and setup guide.",
                    "Warranty terms and support contacts.",
                ],
                "tags": ["technical", "manual", "product"]
            },
        ]
        
        # Ingest documents
        self.log(f"Ingesting {len(documents)} documents with category tags...")
        for doc in documents:
            for i, chunk in enumerate(doc["chunks"]):
                vector = self.ctx.embedder.encode(chunk)
                # Combine document-level tags with chunk position
                keywords = doc["tags"] + [f"chunk_{i}"]
                col.append_block(
                    key=doc["key"],
                    primary=chunk,
                    vector=vector,
                    keywords=keywords
                )
        
        # Test Case 2a: Keyword search only (find all docs with "legal" tag)
        self.log("Test 2a: Keyword-only search for 'legal' documents...")
        legal_keys = col.keyword_search(["legal"])
        self.log(f"  Found {len(legal_keys)} legal document keys: {legal_keys}")
        self.assert_true("contract_alpha.pdf" in legal_keys, "Should find contract_alpha")
        self.assert_true("contract_beta.pdf" in legal_keys, "Should find contract_beta")
        self.assert_true("report_q3.pdf" not in legal_keys, "Should NOT find report_q3")
        
        # Test Case 2b: Combined vector + keyword search
        self.log("Test 2b: Hybrid search (vector + 'financial' keyword)...")
        query = "revenue and income statement"
        query_vector = self.ctx.embedder.encode(query)
        
        # Search with keyword filter
        results = col.search(query_vector, top_k=10, keywords=["financial"])
        result_keys = list(set([res.key for res in results]))
        self.log(f"  Found results from documents: {result_keys}")
        
        # Should only return financial documents
        for key in result_keys:
            self.assert_true(
                "report" in key,
                f"Result '{key}' should be a financial report"
            )
        
        # Test Case 2c: Keyword search for 'confidential' tag
        self.log("Test 2c: Keyword search for 'confidential' documents...")
        confidential_keys = col.keyword_search(["confidential"])
        self.log(f"  Found confidential documents: {confidential_keys}")
        self.assert_equal(len(confidential_keys), 2, "Should find exactly 2 confidential docs")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")


# =============================================================================
# Test 3: Structured Document with Section References
# =============================================================================

class StructuredDocumentSectionTest(BaseDocumentTest):
    """
    Goal: Given a structured document (e.g., legal), chunks should reflect section references.
    
    Test Scenario:
    - Ingest a legal document with sections (1.1, 1.2, 2.1, etc.)
    - When querying, the returned chunks should include section identifiers
    - Verify that users can identify which section the answer came from
    """
    
    def run(self):
        print(f"{bcolors.HEADER}[Test 3] Structured Document Section References{bcolors.ENDC}")
        
        col = self.ctx.create_collection("test_structured_doc", dims=4, metric="l2")
        
        # Simulate a legal document with numbered sections
        legal_document = {
            "key": "master_service_agreement_v2.pdf",
            "sections": [
                {
                    "section_id": "1.0",
                    "title": "Definitions",
                    "content": "In this Agreement, 'Service Provider' means the party providing services, 'Client' means the party receiving services, and 'Effective Date' means the date of signature."
                },
                {
                    "section_id": "1.1",
                    "title": "Service Description",
                    "content": "The Service Provider agrees to deliver consulting services including system design, implementation, and ongoing support."
                },
                {
                    "section_id": "2.0",
                    "title": "Payment Terms",
                    "content": "Client shall pay all invoices within 30 days of receipt. Late payments incur 1.5% monthly interest."
                },
                {
                    "section_id": "2.1",
                    "title": "Expenses",
                    "content": "Travel expenses, accommodation, and other pre-approved costs shall be reimbursed at actual cost plus 10% handling fee."
                },
                {
                    "section_id": "3.0",
                    "title": "Confidentiality",
                    "content": "Both parties agree to maintain confidentiality of all proprietary information shared during the engagement period and for 2 years thereafter."
                },
                {
                    "section_id": "3.1",
                    "title": "Non-Disclosure",
                    "content": "Neither party shall disclose trade secrets, client lists, or business strategies to third parties without written consent."
                },
                {
                    "section_id": "4.0",
                    "title": "Termination",
                    "content": "Either party may terminate this Agreement with 30 days written notice. In case of material breach, immediate termination is permitted."
                },
                {
                    "section_id": "4.1",
                    "title": "Effect of Termination",
                    "content": "Upon termination, Service Provider shall return all client materials and Client shall pay for services rendered up to termination date."
                },
                {
                    "section_id": "5.0",
                    "title": "Liability",
                    "content": "Service Provider's liability is limited to the total fees paid under this Agreement. Neither party is liable for consequential damages."
                },
            ]
        }
        
        # Ingest document with section metadata
        self.log(f"Ingesting structured document with {len(legal_document['sections'])} sections...")
        for section in legal_document["sections"]:
            full_content = f"[Section {section['section_id']}] {section['title']}\n\n{section['content']}"
            vector = self.ctx.embedder.encode(section["content"])
            
            # Use section_id as keyword for precise filtering
            keywords = [
                f"section_{section['section_id'].replace('.', '_')}",
                section['section_id'].split('.')[0],  # Top-level section number
                section['title'].lower().replace(' ', '_')
            ]
            
            col.append_block(
                key=legal_document["key"],
                primary=full_content,
                vector=vector,
                keywords=keywords
            )
        
        # Test Case 3a: Query about termination
        self.log("Test 3a: Query about 'ending the contract early'...")
        query = "How can I end the contract early with notice?"
        query_vector = self.ctx.embedder.encode(query)
        results = col.search(query_vector, top_k=2)
        
        self.assert_true(len(results) > 0, "Should find results")
        found_termination = any("[Section 4.0]" in res.block.primary for res in results)
        self.log(f"  Top result contains section reference: {results[0].block.primary[:80]}...")
        self.assert_true(found_termination, "Should find the Termination section (4.0)")
        
        # Verify section ID is extractable from result
        for res in results:
            section_match = re.search(r'\[Section (\d+\.\d+)\]', res.block.primary)
            self.assert_true(section_match is not None, f"Section ID should be in result: {res.block.primary[:50]}")
            self.log(f"  Found reference to Section {section_match.group(1)}")
        
        # Test Case 3b: Query about payment using keyword filter
        # Note: Since we use a mock embedder (not semantic), we verify via keyword search
        self.log("Test 3b: Keyword search for 'payment_terms' section...")
        keys = col.keyword_search(["payment_terms"])
        self.assert_true(len(keys) > 0, "Should find payment terms section")
        self.log(f"  Found document keys with payment_terms: {keys}")
        
        # Also verify we can retrieve the block with section reference
        block = col.get_block("master_service_agreement_v2.pdf", 2)  # Index 2 is Section 2.0
        found_payment = "[Section 2.0]" in block.primary
        self.assert_true(found_payment, "Block should contain Section 2.0 (Payment Terms)")
        self.log(f"  Found: {block.primary[:80]}...")
        
        # Test Case 3c: Search by section keyword
        self.log("Test 3c: Keyword search for 'confidentiality' section...")
        keys = col.keyword_search(["confidentiality"])
        self.assert_true(len(keys) > 0, "Should find confidentiality sections")
        self.log(f"  Found document keys: {keys}")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")


# =============================================================================
# Test 4: Single Document Query Scope
# =============================================================================

class SingleDocumentQueryTest(BaseDocumentTest):
    """
    Goal: Given a document, ask questions based on that single document only.
    
    Test Scenario:
    - Ingest multiple documents into the same collection
    - Query with a filter that restricts search to a specific document key
    - Verify that results only come from the specified document
    """
    
    def run(self):
        print(f"{bcolors.HEADER}[Test 4] Single Document Query Scope{bcolors.ENDC}")
        
        col = self.ctx.create_collection("test_single_doc", dims=4, metric="l2")
        
        # Two distinct documents with similar content but different contexts
        documents = {
            "company_policy_hr.pdf": [
                {
                    "content": "Annual leave policy: Employees receive 20 days of paid annual leave per calendar year.",
                    "keywords": ["leave", "annual", "policy"]
                },
                {
                    "content": "Sick leave policy: Up to 10 days of paid sick leave are provided per year with medical documentation.",
                    "keywords": ["leave", "sick", "policy"]
                },
                {
                    "content": "Remote work policy: Employees may work remotely up to 3 days per week with manager approval.",
                    "keywords": ["remote", "work", "policy"]
                },
            ],
            "company_policy_it.pdf": [
                {
                    "content": "Password policy: Passwords must be at least 12 characters with uppercase, lowercase, and numbers.",
                    "keywords": ["password", "security", "policy"]
                },
                {
                    "content": "Data backup policy: All work data must be stored on company cloud, not local drives.",
                    "keywords": ["data", "backup", "policy"]
                },
                {
                    "content": "Remote access policy: VPN must be used when accessing company resources from outside the office.",
                    "keywords": ["remote", "access", "vpn", "policy"]
                },
            ],
            "employee_handbook.pdf": [
                {
                    "content": "Leave request process: Submit leave requests through the HR portal at least 2 weeks in advance.",
                    "keywords": ["leave", "request", "process"]
                },
                {
                    "content": "Work hours: Standard work hours are 9 AM to 5 PM with a 1-hour lunch break.",
                    "keywords": ["hours", "schedule", "work"]
                },
            ],
        }
        
        # Ingest all documents
        self.log(f"Ingesting {len(documents)} documents...")
        for doc_key, chunks in documents.items():
            # Add document-level keyword for filtering
            # Keywords can only contain a-z, 0-9, underscore, and dash
            doc_tag = doc_key.replace(".pdf", "").replace(".", "-")
            for i, chunk_data in enumerate(chunks):
                vector = self.ctx.embedder.encode(chunk_data["content"])
                keywords = chunk_data["keywords"] + [doc_tag, f"docid--{doc_tag}"]
                col.append_block(
                    key=doc_key,
                    primary=chunk_data["content"],
                    vector=vector,
                    keywords=keywords
                )
        
        # Test Case 4a: Search within HR policy only
        self.log("Test 4a: Query 'leave policy' scoped to HR document...")
        query = "how many days of leave do employees get?"
        query_vector = self.ctx.embedder.encode(query)
        
        # Filter by document-specific keyword
        results = col.search(query_vector, top_k=5, keywords=["docid--company_policy_hr"])
        
        self.assert_true(len(results) > 0, "Should find results in HR document")
        for res in results:
            self.assert_equal(
                res.key, 
                "company_policy_hr.pdf", 
                f"All results should be from HR document, got: {res.key}"
            )
        self.log(f"  Found {len(results)} chunks from HR policy only")
        self.log(f"  Top result: {results[0].block.primary[:60]}...")
        
        # Test Case 4b: Search within IT policy only
        self.log("Test 4b: Query 'remote' scoped to IT document...")
        query = "remote access requirements"
        query_vector = self.ctx.embedder.encode(query)
        
        results = col.search(query_vector, top_k=5, keywords=["docid--company_policy_it"])
        
        self.assert_true(len(results) > 0, "Should find results in IT document")
        for res in results:
            self.assert_equal(
                res.key, 
                "company_policy_it.pdf", 
                f"All results should be from IT document, got: {res.key}"
            )
        self.log(f"  Found {len(results)} chunks from IT policy only")
        
        # Verify it found VPN-related content (IT policy), not HR remote work
        content_has_vpn = any("VPN" in res.block.primary for res in results)
        self.assert_true(content_has_vpn, "Should find VPN policy from IT document")
        
        # Test Case 4c: Search "leave" in handbook only
        self.log("Test 4c: Query 'leave' scoped to employee handbook...")
        query = "how to request leave"
        query_vector = self.ctx.embedder.encode(query)
        
        results = col.search(query_vector, top_k=5, keywords=["docid--employee_handbook"])
        
        for res in results:
            self.assert_equal(
                res.key,
                "employee_handbook.pdf",
                f"Results should be from handbook only, got: {res.key}"
            )
        self.log(f"  Found {len(results)} chunks from employee handbook")
        
        # Test Case 4d: Compare results with and without scope
        self.log("Test 4d: Compare scoped vs unscoped search for 'remote work'...")
        query = "remote work"
        query_vector = self.ctx.embedder.encode(query)
        
        # Unscoped search
        all_results = col.search(query_vector, top_k=10)
        all_doc_keys = list(set([r.key for r in all_results]))
        self.log(f"  Unscoped: Found results from {len(all_doc_keys)} documents: {all_doc_keys}")
        
        # Scoped to HR only
        hr_results = col.search(query_vector, top_k=10, keywords=["docid--company_policy_hr"])
        hr_doc_keys = list(set([r.key for r in hr_results]))
        self.log(f"  HR-scoped: Found results from {len(hr_doc_keys)} document(s): {hr_doc_keys}")
        
        self.assert_true(
            len(all_doc_keys) >= len(hr_doc_keys),
            "Unscoped should find >= docs than scoped search"
        )
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")


# =============================================================================
# Test 5: Combined Scenario - Multi-Document Legal Search
# =============================================================================

class CombinedLegalSearchTest(BaseDocumentTest):
    """
    Combined test that simulates a real legal document search scenario.
    
    Tests:
    - Multiple legal agreements with sections
    - Finding relevant documents first, then relevant sections
    - Scoping queries to specific contracts
    """
    
    def run(self):
        print(f"{bcolors.HEADER}[Test 5] Combined Legal Document Search{bcolors.ENDC}")
        
        col = self.ctx.create_collection("test_legal_combined", dims=4, metric="l2")
        
        # Three legal contracts with different clients
        contracts = {
            "contract_acme_corp_2024.pdf": {
                "client": "acme",
                "type": "service_agreement",
                "sections": [
                    ("1.0", "Scope", "Acme Corp engages Provider for software development services."),
                    ("2.0", "Payment", "Acme Corp shall pay $50,000 monthly retainer."),
                    ("3.0", "Termination", "Either party may terminate with 60 days notice to Acme Corp."),
                ]
            },
            "contract_globex_2024.pdf": {
                "client": "globex",
                "type": "service_agreement",
                "sections": [
                    ("1.0", "Scope", "Globex Industries requires consulting on AI implementation."),
                    ("2.0", "Payment", "Globex pays $75,000 per milestone delivered."),
                    ("3.0", "Termination", "Globex may terminate for convenience with 30 days notice."),
                ]
            },
            "nda_initech_2024.pdf": {
                "client": "initech",
                "type": "nda",
                "sections": [
                    ("1.0", "Definition", "Confidential Information includes all Initech proprietary data."),
                    ("2.0", "Obligations", "Recipient shall protect Initech information for 5 years."),
                    ("3.0", "Exceptions", "Public information and prior knowledge are excluded."),
                ]
            },
        }
        
        # Ingest with rich metadata
        self.log(f"Ingesting {len(contracts)} legal contracts...")
        for doc_key, meta in contracts.items():
            # Create doc_tag without periods (only a-z, 0-9, underscore, dash allowed)
            doc_tag = doc_key.replace(".pdf", "").replace(".", "-")
            for section_id, title, content in meta["sections"]:
                full_text = f"[Section {section_id}] {title}: {content}"
                vector = self.ctx.embedder.encode(content)
                keywords = [
                    f"client--{meta['client']}",
                    f"type--{meta['type']}",
                    f"section--{section_id.replace('.', '_')}",
                    f"docid--{doc_tag}",
                    meta["client"],
                    meta["type"],
                ]
                col.append_block(
                    key=doc_key,
                    primary=full_text,
                    vector=vector,
                    keywords=keywords
                )
        
        # Scenario A: Find all documents for a specific client
        self.log("Scenario A: Find all Acme Corp documents...")
        acme_keys = col.keyword_search(["client--acme"])
        self.assert_true("contract_acme_corp_2024.pdf" in acme_keys, "Should find Acme contract")
        self.log(f"  Found Acme documents: {acme_keys}")
        
        # Scenario B: Find termination clauses across ALL contracts
        self.log("Scenario B: Search 'termination notice period' across all docs...")
        query = "how much notice is needed to end the contract"
        query_vector = self.ctx.embedder.encode(query)
        results = col.search(query_vector, top_k=10)
        
        doc_terminations = {}
        for res in results:
            if "[Section 3.0]" in res.block.primary or "terminat" in res.block.primary.lower():
                if res.key not in doc_terminations:
                    doc_terminations[res.key] = res.block.primary
        
        self.log(f"  Found termination info in {len(doc_terminations)} contracts:")
        for doc, text in doc_terminations.items():
            self.log(f"    - {doc}: {text[:50]}...")
        
        # Scenario C: Query within a specific contract
        self.log("Scenario C: Query 'payment amount' within Globex contract only...")
        query = "how much do we get paid"
        query_vector = self.ctx.embedder.encode(query)
        results = col.search(query_vector, top_k=5, keywords=["docid--contract_globex_2024"])
        
        self.assert_true(len(results) > 0, "Should find Globex payment section")
        for res in results:
            self.assert_equal(res.key, "contract_globex_2024.pdf", "Only Globex results")
        
        found_75k = any("$75,000" in res.block.primary for res in results)
        self.assert_true(found_75k, "Should find $75,000 milestone payment")
        self.log(f"  Found: {results[0].block.primary}")
        
        # Scenario D: Find all NDAs
        self.log("Scenario D: Find all NDA documents...")
        nda_keys = col.keyword_search(["type--nda"])
        self.assert_true("nda_initech_2024.pdf" in nda_keys, "Should find Initech NDA")
        self.assert_true(len(nda_keys) == 1, "Should only find 1 NDA")
        self.log(f"  Found NDA documents: {nda_keys}")
        
        print(f"{bcolors.OKGREEN}    PASS{bcolors.ENDC}")


# =============================================================================
# Main Test Runner
# =============================================================================

def main():
    print("=" * 60)
    print("Document Retrieval Test Suite for WaddleDB")
    print("=" * 60)
    print()
    
    try:
        ctx = DocumentRetrievalTestContext()
    except Exception as e:
        print(f"{bcolors.FAIL}Failed to connect to WaddleDB: {e}{bcolors.ENDC}")
        print("Please ensure the server is running on localhost:6969")
        sys.exit(1)
    
    tests = [
        TopKDocumentsTest(ctx),
        TopKDocumentsWithKeywordTest(ctx),
        StructuredDocumentSectionTest(ctx),
        SingleDocumentQueryTest(ctx),
        CombinedLegalSearchTest(ctx),
    ]
    
    print(f"Running {len(tests)} tests...\n")
    print("-" * 60)
    
    passed = 0
    failed = 0
    
    for test in tests:
        try:
            test.run()
            passed += 1
        except AssertionError as e:
            print(f"{bcolors.FAIL}    FAIL: {e}{bcolors.ENDC}")
            failed += 1
        except Exception as e:
            print(f"{bcolors.FAIL}    ERROR: {e}{bcolors.ENDC}")
            import traceback
            traceback.print_exc()
            failed += 1
        print("-" * 60)
    
    ctx.cleanup()
    
    print()
    print("=" * 60)
    if failed == 0:
        print(f"{bcolors.OKGREEN}ALL {passed} TESTS PASSED{bcolors.ENDC}")
        sys.exit(0)
    else:
        print(f"{bcolors.FAIL}{failed} TESTS FAILED, {passed} PASSED{bcolors.ENDC}")
        sys.exit(1)


if __name__ == "__main__":
    main()
