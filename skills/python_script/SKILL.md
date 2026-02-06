---
name: PythonScript
description: 執行特定的 Python 數據處理腳本。
command: docker run --rm -v $(pwd):/app python:3.9 python /app/script.py "{{args}}"
---
# PythonScript

Execute python scripts.
