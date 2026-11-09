const http = require('http');
const fs = require('fs');
const path = require('path');

const docsDir = path.resolve(__dirname);
const PORT = 9876;

// Serve static files from docs dir
const server = http.createServer((req, res) => {
    let filePath = path.join(docsDir, req.url === '/' ? 'index.html' : req.url);
    const ext = path.extname(filePath);
    const mimeTypes = {
        '.html': 'text/html',
        '.js': 'application/javascript',
        '.css': 'text/css',
        '.json': 'application/json',
        '.png': 'image/png',
        '.jpg': 'image/jpeg',
    };
    
    fs.readFile(filePath, (err, content) => {
        if (err) {
            res.writeHead(404);
            res.end('Not found');
            return;
        }
        res.writeHead(200, { 'Content-Type': mimeTypes[ext] || 'text/plain' });
        res.end(content);
    });
});

server.listen(PORT, () => {
    console.log(`Server running at http://localhost:${PORT}`);
    console.log('HTML files available at:');
    fs.readdirSync(docsDir)
        .filter(f => f.endsWith('.html') && f !== 'swagger-ui.html')
        .forEach(f => console.log(`  http://localhost:${PORT}/${f}`));
    console.log('\nServer will run until killed. Use Ctrl+C to stop.');
});
