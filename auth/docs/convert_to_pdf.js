const fs = require('fs');
const path = require('path');

// Simple markdown to HTML converter (no external deps needed - we'll use marked via npx)
const docsDir = __dirname;

// Get all .md files
const mdFiles = fs.readdirSync(docsDir).filter(f => f.endsWith('.md'));

console.log(`Found ${mdFiles.length} markdown files to convert:`);
mdFiles.forEach(f => console.log(`  - ${f}`));

// We'll generate HTML files that can be opened in browser and printed to PDF
mdFiles.forEach(file => {
    const mdContent = fs.readFileSync(path.join(docsDir, file), 'utf-8');
    const htmlFile = file.replace('.md', '.html');
    const title = file.replace('.md', '').replace(/_/g, ' ').replace(/-/g, ' ');
    
    const htmlContent = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>${title}</title>
<style>
  @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Fira+Code:wght@400;500&display=swap');
  
  * { margin: 0; padding: 0; box-sizing: border-box; }
  
  body {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    line-height: 1.7;
    color: #1a1a2e;
    background: #fff;
    padding: 40px 60px;
    max-width: 900px;
    margin: 0 auto;
    font-size: 14px;
  }
  
  h1 { font-size: 28px; font-weight: 700; color: #0f0f23; margin: 30px 0 15px; border-bottom: 3px solid #4361ee; padding-bottom: 10px; }
  h2 { font-size: 22px; font-weight: 600; color: #16213e; margin: 25px 0 12px; border-bottom: 1px solid #e0e0e0; padding-bottom: 8px; }
  h3 { font-size: 17px; font-weight: 600; color: #1a1a2e; margin: 20px 0 10px; }
  h4 { font-size: 15px; font-weight: 600; color: #333; margin: 15px 0 8px; }
  
  p { margin: 8px 0; }
  
  strong { color: #0f0f23; }
  
  code {
    font-family: 'Fira Code', 'Consolas', 'Monaco', monospace;
    background: #f4f4f8;
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 12.5px;
    color: #e74c3c;
  }
  
  pre {
    background: #1e1e2e;
    color: #cdd6f4;
    padding: 16px 20px;
    border-radius: 8px;
    overflow-x: auto;
    margin: 12px 0;
    font-size: 12.5px;
    line-height: 1.5;
    border: 1px solid #313244;
  }
  
  pre code {
    background: none;
    color: inherit;
    padding: 0;
    font-size: inherit;
  }
  
  blockquote {
    border-left: 4px solid #4361ee;
    padding: 10px 20px;
    margin: 15px 0;
    background: #f8f9ff;
    border-radius: 0 6px 6px 0;
    color: #444;
  }
  
  table {
    width: 100%;
    border-collapse: collapse;
    margin: 15px 0;
    font-size: 13px;
  }
  
  th {
    background: #4361ee;
    color: #fff;
    padding: 10px 14px;
    text-align: left;
    font-weight: 600;
    font-size: 12.5px;
  }
  
  td {
    padding: 9px 14px;
    border-bottom: 1px solid #e8e8e8;
  }
  
  tr:nth-child(even) td { background: #f8f9fa; }
  tr:hover td { background: #eef1ff; }
  
  ul, ol { margin: 8px 0; padding-left: 25px; }
  li { margin: 4px 0; }
  
  hr {
    border: none;
    border-top: 2px solid #e0e0e0;
    margin: 30px 0;
  }
  
  a { color: #4361ee; text-decoration: none; }
  a:hover { text-decoration: underline; }
  
  /* Mermaid diagrams - show as code in PDF */
  .mermaid-placeholder {
    background: #f0f4ff;
    border: 2px dashed #4361ee;
    border-radius: 8px;
    padding: 20px;
    margin: 15px 0;
    text-align: center;
    color: #4361ee;
    font-style: italic;
  }
  
  @media print {
    body { padding: 20px 30px; font-size: 12px; }
    pre { page-break-inside: avoid; }
    table { page-break-inside: avoid; }
    h1, h2, h3 { page-break-after: avoid; }
    hr { page-break-after: avoid; }
  }
</style>
<script>
// We'll use marked.js CDN for rendering
</script>
</head>
<body>
<div id="content"></div>
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<script>
const md = ${JSON.stringify(mdContent)};
document.getElementById('content').innerHTML = marked.parse(md);
</script>
</body>
</html>`;
    
    const outputPath = path.join(docsDir, htmlFile);
    // Don't overwrite existing HTML files like swagger-ui.html
    if (file === 'swagger-ui.html') return;
    
    fs.writeFileSync(outputPath, htmlContent, 'utf-8');
    console.log(`✅ Created: ${htmlFile}`);
});

console.log('\n📄 HTML files created. Opening in browser for PDF printing...');
console.log('To convert to PDF: Open each HTML file in browser → Ctrl+P → Save as PDF');
