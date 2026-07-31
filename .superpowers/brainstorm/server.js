const http = require('http');
const fs = require('fs');
const path = require('path');

const PORT = 52341;
const CONTENT_DIR = path.join(__dirname, 'content');
const STATE_DIR = path.join(__dirname, 'state');

// Ensure state dir
if (!fs.existsSync(STATE_DIR)) fs.mkdirSync(STATE_DIR, { recursive: true });

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css',
  '.js': 'application/javascript',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
};

function serveFile(res, filePath, mimeType) {
  try {
    const content = fs.readFileSync(filePath);
    res.writeHead(200, { 'Content-Type': mimeType });
    res.end(content);
  } catch {
    res.writeHead(404);
    res.end('Not Found');
  }
}

function getNewestFile(dir) {
  try {
    const files = fs.readdirSync(dir)
      .filter(f => f.endsWith('.html'))
      .map(f => ({ name: f, mtime: fs.statSync(path.join(dir, f)).mtimeMs }))
      .sort((a, b) => b.mtime - a.mtime);
    return files.length > 0 ? files[0].name : null;
  } catch { return null; }
}

const FRAME_TEMPLATE = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Brainstorm - B</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #1a1a2e; color: #e0e0e0; padding: 2rem; min-height: 100vh;
  }
  .container { max-width: 1100px; margin: 0 auto; }
  h2 { font-size: 1.5rem; font-weight: 600; color: #fff; margin-bottom: 0.5rem; }
  .subtitle { color: #888; margin-bottom: 1.5rem; font-size: 0.95rem; }
  .section { margin-bottom: 2rem; }
  .options { display: flex; flex-direction: column; gap: 0.75rem; }
  .option {
    background: #16213e; border: 1px solid #2a2a4a; border-radius: 8px;
    padding: 1rem 1.25rem; cursor: pointer; transition: all 0.2s;
    display: flex; align-items: flex-start; gap: 1rem;
  }
  .option:hover { border-color: #4a4a7a; background: #1a2744; }
  .option.selected { border-color: #6c63ff; background: #1f2a50; }
  .option .letter {
    width: 28px; height: 28px; border-radius: 50%; background: #2a2a4a;
    display: flex; align-items: center; justify-content: center;
    font-weight: 600; font-size: 0.85rem; color: #aaa; flex-shrink: 0;
  }
  .option.selected .letter { background: #6c63ff; color: #fff; }
  .option .content h3 { font-size: 1rem; color: #fff; margin-bottom: 0.2rem; }
  .option .content p { font-size: 0.85rem; color: #999; line-height: 1.4; }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 1rem; }
  .card {
    background: #16213e; border: 1px solid #2a2a4a; border-radius: 8px;
    overflow: hidden; cursor: pointer; transition: all 0.2s;
  }
  .card:hover { border-color: #4a4a7a; }
  .card.selected { border-color: #6c63ff; }
  .card-image { background: #0f3460; padding: 1.5rem; min-height: 120px; display: flex; align-items: center; justify-content: center; }
  .card-body { padding: 1rem; }
  .card-body h3 { font-size: 1rem; color: #fff; margin-bottom: 0.3rem; }
  .card-body p { font-size: 0.85rem; color: #999; }
  .mockup {
    background: #16213e; border: 1px solid #2a2a4a; border-radius: 8px; overflow: hidden;
  }
  .mockup-header {
    background: #0f3460; padding: 0.6rem 1rem; font-size: 0.8rem; color: #888;
    border-bottom: 1px solid #2a2a4a;
  }
  .mockup-body { padding: 1rem; }
  .mock-nav { background: #0f3460; padding: 0.5rem 1rem; border-radius: 4px; font-size: 0.8rem; color: #aaa; display: flex; gap: 1rem; }
  .mock-sidebar { background: #0f3460; padding: 0.75rem; border-radius: 4px; min-width: 120px; font-size: 0.75rem; color: #888; }
  .mock-content { background: #16213e; padding: 1rem; border-radius: 4px; flex: 1; min-height: 200px; border: 1px dashed #2a2a4a; font-size: 0.8rem; color: #666; }
  .mock-button { background: #6c63ff; color: #fff; border: none; padding: 0.5rem 1rem; border-radius: 6px; cursor: pointer; font-size: 0.85rem; }
  .mock-input { background: #0f3460; border: 1px solid #2a2a4a; color: #e0e0e0; padding: 0.5rem 0.75rem; border-radius: 6px; font-size: 0.85rem; width: 100%; }
  .placeholder { background: #0f3460; border: 1px dashed #2a2a4a; border-radius: 4px; padding: 2rem; text-align: center; color: #555; font-size: 0.85rem; }
  .pros-cons { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
  .pros h4, .cons h4 { font-size: 0.9rem; margin-bottom: 0.5rem; }
  .pros h4 { color: #4caf50; } .cons h4 { color: #f44336; }
  .pros ul, .cons ul { list-style: none; }
  .pros li, .cons li { font-size: 0.85rem; color: #bbb; padding: 0.2rem 0; }
  .pros li::before { content: "✓ "; color: #4caf50; }
  .cons li::before { content: "✗ "; color: #f44336; }
  .split { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
  .label { font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.1em; color: #666; margin-bottom: 0.5rem; }
  .diagram-box {
    background: #0f3460; border: 1px solid #2a2a4a; border-radius: 8px; padding: 1rem; margin: 0.5rem 0; text-align: center;
  }
  .diagram-box.highlight { border-color: #6c63ff; background: #1f2a50; }
  .diagram-box h4 { font-size: 0.85rem; color: #fff; margin-bottom: 0.3rem; }
  .diagram-box p { font-size: 0.75rem; color: #888; }
  .arrow-down { text-align: center; color: #4a4a7a; font-size: 1.2rem; padding: 0.25rem 0; }
  .arrow-right { color: #4a4a7a; font-size: 1rem; padding: 0 0.5rem; }
  .flex-row { display: flex; align-items: center; justify-content: center; gap: 0.5rem; }
  .flex-wrap { display: flex; flex-wrap: wrap; gap: 0.75rem; justify-content: center; }
  .tech-tag {
    display: inline-block; background: #1a2744; color: #6c63ff; padding: 0.2rem 0.6rem;
    border-radius: 4px; font-size: 0.75rem; border: 1px solid #2a2a5a;
  }
  .status-bar { display: flex; justify-content: center; gap: 1rem; margin-top: 1rem; font-size: 0.75rem; color: #555; }
  .status-bar .connected { color: #4caf50; }
</style>
</head>
<body>
<div class="container">
`;

const FRAME_CLOSE = `
<div class="status-bar">
  <span class="connected">● connected</span>
  <span>server:52341</span>
</div>
</div>
<script>
// Enable option selection
window.toggleSelect = function(el) {
  const parent = el.closest('.options, .cards');
  if (parent && !parent.hasAttribute('data-multiselect')) {
    parent.querySelectorAll('.selected').forEach(s => s.classList.remove('selected'));
  }
  el.classList.toggle('selected');
  // Send event
  const choice = el.dataset.choice || '';
  const text = el.querySelector('h3')?.textContent || '';
  fetch('/event', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({type:'click', choice, text, timestamp: Date.now()/1000})
  }).catch(() => {});
};
// Make option / card onclick work
document.querySelectorAll('.option, .card').forEach(el => {
  if (!el.hasAttribute('onclick')) {
    el.addEventListener('click', () => window.toggleSelect(el));
  }
});
</script>
</body>
</html>`;

const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`);

  if (url.pathname === '/event' && req.method === 'POST') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      try {
        const eventFile = path.join(STATE_DIR, 'events');
        fs.appendFileSync(eventFile, body + '\n');
      } catch {}
      res.writeHead(200);
      res.end('ok');
    });
    return;
  }

  if (url.pathname === '/state') {
    const events = [];
    try {
      const data = fs.readFileSync(path.join(STATE_DIR, 'events'), 'utf8');
      data.trim().split('\n').forEach(l => { try { events.push(JSON.parse(l)); } catch {} });
    } catch {}
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ events }));
    return;
  }

  // Serve content
  const newest = getNewestFile(CONTENT_DIR);
  if (newest) {
    const contentPath = path.join(CONTENT_DIR, newest);
    const raw = fs.readFileSync(contentPath, 'utf8');
    if (raw.trim().startsWith('<!DOCTYPE') || raw.trim().startsWith('<html')) {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(raw);
    } else {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(FRAME_TEMPLATE + raw + FRAME_CLOSE);
    }
  } else {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(FRAME_TEMPLATE + '<p class="subtitle">等待内容...</p>' + FRAME_CLOSE);
  }
});

server.listen(PORT, () => {
  console.log(JSON.stringify({
    type: 'server-started',
    port: PORT,
    url: `http://localhost:${PORT}`,
    screen_dir: CONTENT_DIR,
    state_dir: STATE_DIR
  }));
});
