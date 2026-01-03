1. Semantic Search (Concept Matching)

These queries do not use exact keywords but should find the relevant sections based on meaning.

Query: "How does the system handle bad weather or snow?"

Target Block: Section 5 (Risk Analysis) - Mentions LiDAR accuracy and snow.

Query: "What is the battery life and range of the delivery robots?"

Target Block: Section 3 (Hardware Specifications) - Mentions 60km range.

Query: "Cost breakdown for the manufacturing phase"

Target Block: Section 4 (Financial Overview) - Mentions the $7M fleet cost.

Query: "AI routing and cloud control"

Target Block: Section 2.3 (The Core Layer/Hive).

2. Fact Retrieval (Specific Details)

These queries look for specific facts embedded in the text.

Query: "Project Omega launch date"

Target Block: Section 1 (Executive Summary) - Mentions Q3 2025.

Query: "Maximum payload capacity"

Target Block: Section 2.1 (Physical Layer) - Mentions 50kg cargo bay.

Query: "Encryption and security protocols"

Target Block: Section 5 (Risk Analysis) - Mentions AES-256.

3. Keyword Search (Testing Filters)

If you implement the keyword filtering logic, use these to test specific terms.

Keywords: ["lidar", "sensors"]

Target Block: Section 3 (Hardware Specs) & Section 2.1.

Keywords: ["budget", "finance"]

Target Block: Section 4 (Financial Overview).