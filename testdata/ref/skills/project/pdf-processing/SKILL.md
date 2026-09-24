---
name: pdf-processing
description: Extract text and tables from PDF files, fill forms, merge documents. Use when a task mentions PDFs or .pdf files.
license: Apache-2.0
compatibility: Requires Python 3.13+ and qpdf on the PATH.
allowed-tools: Bash(git:*) Bash(jq:*) Read
metadata:
  author: example-org
  version: "1.0"
---

# PDF processing

Use this skill when the task involves a PDF.

1. Read `references/REFERENCE.md` for the field names.
2. Run `scripts/extract.py` on the file.
3. The layout of a filled form is in `assets/diagram.png`.
