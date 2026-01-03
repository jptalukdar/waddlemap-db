# About Dataset

Source: https://www.kaggle.com/datasets/samuelmatsuoharris/single-topic-rag-evaluation-dataset/data

## What is this dataset?

This dataset was designed to evaluate the performance of RAG AI querying text documents about a single topic with word counts ranging from a few thousand to a few tens of thousands, such as articles, blogs, and documentation. The sources were intentionally chosen to have been produced within the last few years (from the time of writing in July 2024) and to be relatively niche, to reduce the chance of evaluated LLMs including this information in their training datasets.

There are 120 question-answer pairs in this dataset.

In this dataset, there are:
* **40 questions** that do not have an answer within the document.
* **40 question-answer pairs** that have an answer that must be generated from a single passage of the document.
* **40 question-answer pairs** that have an answer that must be generated from multiple passages of the document.

The answers to the questions with no answer within the text are intended to be some variation of "I do not know". The exact expected answer can be decided by the user of this dataset.

This dataset consists of 20 text documents with 6 questions-answer pairs per document. For each document:
* **2 questions** do not have an answer within the text.
* **2 questions** have an answer that must be generated from a single passage of the document.
* **2 questions** have an answer that must be generated from multiple passages of the document.

## Why was this dataset created?

This dataset was created for my STICI-note AI that you can read about in my blog [here](#) and the code for it can be found [here](#). I created this dataset because I could not find a dataset that could properly evaluate my RAG system. The RAG evaluation datasets that I found would either: 
1. Evaluate a RAG system with text chunks from many varying topics from marine biology to history.
2. Evaluate whether only the retriever in the RAG system worked.
3. Use Wikipedia. 

The variability in topics was an issue because my RAG system was intended to answer queries on text documents that are entirely about a single topic such as documentation on a repo or a notes made about a subject the user is learning about. I wanted to evaluate my AI system as a whole instead of just the retriever, which made datasets for testing whether the correct chunk of text was fetched irrelevant to my use-case. Wikipedia being the source was an issue because Wikipedia is used to train most LLMs, making data leakage a serious concern when using pre-trained models like I was.