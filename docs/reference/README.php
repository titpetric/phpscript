<?php

declare(strict_types=1);

/**
 * Generate the reference sitemap from README.yml and headings discovered in
 * every Markdown document below this directory.
 *
 * Usage: php README.php > README.md
 */

$root = __DIR__;
$config = parseConfig($root . '/README.yml');
$documents = discoverDocuments($root);

echo renderSitemap($config, $documents);

/** @return array{title: string, description: string, compatibility: list<array<string, string>>, sitemap: list<array<string, mixed>>} */
function parseConfig(string $path): array
{
    $lines = file($path, FILE_IGNORE_NEW_LINES);
    if ($lines === false) {
        throw new RuntimeException("Unable to read {$path}");
    }

    $config = [
        'title' => '',
        'description' => '',
        'compatibility' => [],
        'sitemap' => [],
    ];
    $mode = '';
    $item = -1;
    $link = -1;

    foreach ($lines as $number => $line) {
        if ($line === '' || str_starts_with(ltrim($line), '#')) {
            continue;
        }

        if (preg_match('/^(title|description):\s*(.+)$/', $line, $match)) {
            $config[$match[1]] = yamlScalar($match[2]);
            continue;
        }
        if ($line === 'compatibility:') {
            $mode = 'compatibility';
            $item = -1;
            continue;
        }
        if ($line === 'sitemap:') {
            $mode = 'sitemap';
            $item = -1;
            continue;
        }

        if ($mode === 'compatibility') {
            if (preg_match('/^  - feature:\s*(.+)$/', $line, $match)) {
                $config['compatibility'][] = ['feature' => yamlScalar($match[1])];
                $item++;
                continue;
            }
            if ($item >= 0 && preg_match('/^    (status|notes):\s*(.+)$/', $line, $match)) {
                $config['compatibility'][$item][$match[1]] = yamlScalar($match[2]);
                continue;
            }
        }

        if ($mode === 'sitemap') {
            if (preg_match('/^  - section:\s*(.+)$/', $line, $match)) {
                $config['sitemap'][] = [
                    'section' => yamlScalar($match[1]),
                    'description' => '',
                    'links' => [],
                ];
                $item++;
                $link = -1;
                continue;
            }
            if ($item >= 0 && preg_match('/^    description:\s*(.+)$/', $line, $match)) {
                $config['sitemap'][$item]['description'] = yamlScalar($match[1]);
                continue;
            }
            if ($item >= 0 && preg_match('/^      - path:\s*(.+)$/', $line, $match)) {
                $config['sitemap'][$item]['links'][] = [
                    'path' => yamlScalar($match[1]),
                    'description' => '',
                ];
                $link++;
                continue;
            }
            if ($item >= 0 && $link >= 0 && preg_match('/^        description:\s*(.+)$/', $line, $match)) {
                $config['sitemap'][$item]['links'][$link]['description'] = yamlScalar($match[1]);
                continue;
            }
            if (preg_match('/^    links:\s*$/', $line)) {
                continue;
            }
        }

        throw new RuntimeException(sprintf('Unsupported README.yml syntax on line %d: %s', $number + 1, $line));
    }

    if ($config['title'] === '' || $config['sitemap'] === []) {
        throw new RuntimeException('README.yml must define title and sitemap');
    }

    return $config;
}

function yamlScalar(string $value): string
{
    $value = trim($value);
    if (strlen($value) >= 2) {
        $first = $value[0];
        $last = $value[strlen($value) - 1];
        if (($first === '"' && $last === '"') || ($first === "'" && $last === "'")) {
            return substr($value, 1, -1);
        }
    }
    return $value;
}

/** @return array<string, array{title: string, headings: list<array{level: int, text: string, anchor: string}>}> */
function discoverDocuments(string $root): array
{
    $documents = [];
    $iterator = new RecursiveIteratorIterator(new RecursiveDirectoryIterator($root));

    foreach ($iterator as $file) {
        if (!$file->isFile() || strtolower($file->getExtension()) !== 'md') {
            continue;
        }
        $path = str_replace('\\', '/', substr($file->getPathname(), strlen($root) + 1));
        if ($path === 'README.md') {
            continue;
        }
        $documents[$path] = readHeadings($file->getPathname());
    }

    ksort($documents);
    return $documents;
}

/** @return array{title: string, headings: list<array{level: int, text: string, anchor: string}>} */
function readHeadings(string $path): array
{
    $lines = file($path, FILE_IGNORE_NEW_LINES);
    if ($lines === false) {
        throw new RuntimeException("Unable to read {$path}");
    }

    $title = '';
    $headings = [];
    $inFence = false;
    $anchors = [];

    foreach ($lines as $line) {
        if (preg_match('/^\s*(```|~~~)/', $line)) {
            $inFence = !$inFence;
            continue;
        }
        if ($inFence || !preg_match('/^(#{1,6})\s+(.+?)\s*#*\s*$/', $line, $match)) {
            continue;
        }

        $level = strlen($match[1]);
        $text = trim($match[2]);
        $base = headingAnchor($text);
        $count = $anchors[$base] ?? 0;
        $anchors[$base] = $count + 1;
        $anchor = $count === 0 ? $base : $base . '-' . $count;
        if ($level === 1 && $title === '') {
            $title = $text;
            continue;
        }
        if ($level < 2) {
            continue;
        }

        $headings[] = ['level' => $level, 'text' => $text, 'anchor' => $anchor];
    }

    if ($title === '') {
        throw new RuntimeException("Markdown document has no level-one heading: {$path}");
    }

    return ['title' => $title, 'headings' => $headings];
}

function headingAnchor(string $heading): string
{
    $heading = preg_replace('/<[^>]+>/', '', $heading) ?? $heading;
    $heading = str_replace('`', '', $heading);
    $heading = mb_strtolower(html_entity_decode($heading, ENT_QUOTES | ENT_HTML5, 'UTF-8'));
    $heading = preg_replace('/[^\p{L}\p{N}\s_-]/u', '', $heading) ?? $heading;
    return trim(preg_replace('/\s+/u', '-', $heading) ?? $heading, '-');
}

/** @param array<string, mixed> $config @param array<string, array{title: string, headings: list<array{level: int, text: string, anchor: string}>}> $documents */
function renderSitemap(array $config, array $documents): string
{
    $used = [];
    $output = '# ' . $config['title'] . "\n\n";
    $output .= "| Reference area | Status | Notes |\n| --- | --- | --- |\n";
    foreach ($config['compatibility'] as $row) {
        $output .= sprintf("| %s | %s | %s |\n", $row['feature'], $row['status'], $row['notes']);
    }
    $output .= "\n" . $config['description'] . "\n\n";
    $output .= "This file is generated by `php README.php > README.md`. Edit `README.yml` to change ordering or descriptions; document labels and nested links are discovered from Markdown headings.\n\n";

    foreach ($config['sitemap'] as $section) {
        $output .= '## ' . $section['section'] . "\n\n" . $section['description'] . "\n\n";
        foreach ($section['links'] as $link) {
            $path = $link['path'];
            if (!isset($documents[$path])) {
                throw new RuntimeException("README.yml references missing Markdown document: {$path}");
            }
            $used[$path] = true;
            $document = $documents[$path];
            $output .= sprintf("- [%s](%s) — %s\n", $document['title'], $path, $link['description']);
            foreach ($document['headings'] as $heading) {
                $indent = str_repeat('  ', max(1, $heading['level'] - 1));
                $output .= sprintf("%s- [%s](%s#%s)\n", $indent, $heading['text'], $path, $heading['anchor']);
            }
        }
        $output .= "\n";
    }

    $unlisted = array_diff_key($documents, $used);
    if ($unlisted !== []) {
        throw new RuntimeException('README.yml does not list discovered Markdown documents: ' . implode(', ', array_keys($unlisted)));
    }

    return rtrim($output) . "\n";
}
