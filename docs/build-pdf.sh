#!/usr/bin/env bash
# 產生 docs/SyncCraft-Guide.md（合併版）與 docs/SyncCraft-Guide.pdf
set -euo pipefail
cd "$(dirname "$0")"
OUT_MD=SyncCraft-Guide.md
OUT_HTML=pdf/SyncCraft-Guide.html
OUT_PDF=SyncCraft-Guide.pdf

# 合併各部 Markdown
: > "$OUT_MD"
for f in guide/[0-9][0-9]-*.md; do cat "$f" >> "$OUT_MD"; printf '\n\n' >> "$OUT_MD"; done

pandoc "$OUT_MD" -f gfm+attributes -t html5 -s --toc --toc-depth=2 \
  --metadata title="SyncCraft 實作教材" --metadata lang=zh-TW \
  --variable title= --highlight-style=pygments \
  --css style.css --include-before-body=pdf/cover.html \
  -o "$OUT_HTML"
# pandoc 的預設 title block 不需要（封面已有）
sed -i '/<header id="title-block-header">/,/<\/header>/d' "$OUT_HTML"

if command -v weasyprint >/dev/null 2>&1 && weasyprint --version >/dev/null 2>&1; then
  weasyprint "$OUT_HTML" "$OUT_PDF"
else
  uvx --from weasyprint weasyprint "$OUT_HTML" "$OUT_PDF"
fi
echo "✓ $OUT_MD"
echo "✓ $OUT_PDF"
