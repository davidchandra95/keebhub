import {existsSync, readdirSync, readFileSync, statSync} from 'node:fs';
import {dirname, extname, join, relative, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const errors = [];
const ignoredDirectories = new Set(['.git', 'dist', 'node_modules']);

function walk(directory) {
  return readdirSync(directory, {withFileTypes: true}).flatMap((entry) => {
    if (entry.isDirectory() && ignoredDirectories.has(entry.name)) {
      return [];
    }
    const fullPath = join(directory, entry.name);
    return entry.isDirectory() ? walk(fullPath) : [fullPath];
  });
}

const files = walk(root);
const textExtensions = new Set([
  '', '.go', '.html', '.json', '.md', '.mjs', '.ts', '.tsx', '.yaml', '.yml',
]);

for (const file of files) {
  const projectPath = relative(root, file);
  if (projectPath === 'scripts/check-docs.mjs' || !textExtensions.has(extname(file))) {
    continue;
  }
  const source = readFileSync(file, 'utf8');
  if (/KeebMarket|keebmarket|keeb-market/.test(source)) {
    errors.push(`${projectPath}: contains a legacy project name`);
  }
}

for (const file of files.filter((path) => extname(path) === '.md')) {
  const source = readFileSync(file, 'utf8');
  const links = source.matchAll(/!?\[[^\]]*\]\(([^)\s]+)(?:\s+["'][^)]*["'])?\)/g);
  for (const match of links) {
    const rawTarget = match[1].replace(/^<|>$/g, '');
    if (/^(?:[a-z]+:|#)/i.test(rawTarget)) {
      continue;
    }
    const pathPart = decodeURIComponent(rawTarget.split('#', 1)[0].split('?', 1)[0]);
    const target = resolve(dirname(file), pathPart);
    if (!existsSync(target)) {
      errors.push(`${relative(root, file)}: broken relative link ${rawTarget}`);
    }
  }
}

for (const file of files) {
  const projectPath = relative(root, file);
  if (file.endsWith('.DS_Store') || projectPath === 'docs/keeb-market-project-docs.zip') {
    errors.push(`${projectPath}: generated or duplicate file must not be present`);
  }
}

const apiDoc = readFileSync(join(root, 'docs/07-api-contract.md'), 'utf8');
const documentedOperations = new Set(
  [...apiDoc.matchAll(/^###\s+(GET|POST|PATCH|PUT|DELETE)\s+`([^`]+)`/gm)].map(
    ([, method, path]) => `${method.toLowerCase()} ${path.split('?', 1)[0]}`,
  ),
);

const openapi = readFileSync(join(root, 'api/openapi.yaml'), 'utf8');
const contractOperations = new Set();
let currentPath = '';
for (const line of openapi.split('\n')) {
  const pathMatch = line.match(/^  (\/[^:]+):$/);
  if (pathMatch) {
    currentPath = pathMatch[1];
    continue;
  }
  const methodMatch = line.match(/^    (get|post|patch|put|delete):$/);
  if (currentPath && methodMatch) {
    contractOperations.add(`${methodMatch[1]} ${currentPath}`);
  }
}

for (const operation of documentedOperations) {
  if (!contractOperations.has(operation)) {
    errors.push(`docs/07-api-contract.md: ${operation} is missing from api/openapi.yaml`);
  }
}
for (const operation of contractOperations) {
  if (!documentedOperations.has(operation)) {
    errors.push(`api/openapi.yaml: ${operation} is missing from docs/07-api-contract.md`);
  }
}

if (documentedOperations.size === 0 || contractOperations.size === 0) {
  errors.push('API operation agreement check found no operations');
}

if (errors.length > 0) {
  console.error(errors.map((error) => `- ${error}`).join('\n'));
  process.exitCode = 1;
} else {
  console.log(
    `Documentation checks passed (${documentedOperations.size} HTTP operations, no broken links or legacy names).`,
  );
}
