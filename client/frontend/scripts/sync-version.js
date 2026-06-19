#!/usr/bin/env node

const fs = require('fs');

const version = process.argv[2];
if (!version) {
  console.error('Usage: node scripts/sync-version.js <version>');
  process.exit(1);
}

function updateJson(file, update) {
  if (!fs.existsSync(file)) return;
  const data = JSON.parse(fs.readFileSync(file, 'utf8'));
  update(data);
  fs.writeFileSync(file, JSON.stringify(data, null, 2) + '\n');
}

updateJson('package.json', data => {
  data.version = version;
});

updateJson('package-lock.json', data => {
  data.version = version;
  if (data.packages && data.packages['']) {
    data.packages[''].version = version;
  }
});

updateJson('src-tauri/tauri.conf.json', data => {
  data.version = version;
});

const cargoFile = 'src-tauri/Cargo.toml';
if (fs.existsSync(cargoFile)) {
  const cargo = fs
    .readFileSync(cargoFile, 'utf8')
    .replace(/^version = "[^"]+"/m, `version = "${version}"`);
  fs.writeFileSync(cargoFile, cargo);
}

const indexFile = 'public/index.html';
if (fs.existsSync(indexFile)) {
  const html = fs
    .readFileSync(indexFile, 'utf8')
    .replace(/WireSocket \d+\.\d+\.\d+/g, `WireSocket ${version}`);
  fs.writeFileSync(indexFile, html);
}
