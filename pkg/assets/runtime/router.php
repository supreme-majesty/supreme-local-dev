<?php

/**
 * Supreme Local Dev - Router
 * 
 * This script is the entry point for all requests.
 * It routes requests to the appropriate project based on the hostname.
 */

// 1. Identify the host
$host = $_SERVER['HTTP_HOST'];
$tld = 'test'; // To be loaded from state in real impl
$domain = preg_replace('/\.test$/', '', $host);

// Load configuration (state path)
$configPath = __DIR__ . '/config.inc.php';
$statePath = null;
if (file_exists($configPath)) {
    include $configPath; // Should define $sld_state_path
    if (isset($sld_state_path)) {
        $statePath = $sld_state_path;
    }
}

if (!$statePath) {
    // Fallback or error
    $home = getenv('HOME') ?: '/root';
    $statePath = $home . '/.sld/state.json';
}

if (!file_exists($statePath)) {
    http_response_code(404);
    echo "SLD State not found at $statePath";
    exit;
}

$state = json_decode(file_get_contents($statePath), true);
$paths = $state['paths'] ?? [];
$links = $state['links'] ?? [];

$projectPath = null;

// 2. Handle Localhost & Tools
if ($domain === 'localhost') {
    $uri = $_SERVER['REQUEST_URI'];

    // Tool: PHPMyAdmin
    if (strpos($uri, '/phpmyadmin') === 0) {
        $possiblePaths = [
            '/usr/share/phpmyadmin',
            '/opt/lampp/phpmyadmin',
        ];

        $toolPath = null;
        foreach ($possiblePaths as $p) {
            if (is_dir($p)) {
                $toolPath = $p;
                break;
            }
        }

        if ($toolPath) {
            // Fix URI for the tool
            // We need to serve this directory.
            $projectPath = $toolPath;
            // Adjust URI to be relative if needed, but for now let's just treat it as a project
        } else {
            http_response_code(404);
            echo "PHPMyAdmin not found. Checked: " . implode(", ", $possiblePaths);
            exit;
        }
    } else {
        // Dashboard
        echo "<h1>Supreme Local Dev 🚀</h1>";
        echo "<p><strong>Global State:</strong> $statePath</p>";
        echo "<h2>Parked Paths</h2><ul>";
        foreach ($paths as $p)
            echo "<li>$p</li>";
        echo "</ul>";
        echo "<h2>Linked Sites</h2><ul>";
        foreach ($links as $s => $p)
            echo "<li><a href='http://$s.$tld'>$s.$tld</a> -> $p</li>";
        echo "</ul>";
        exit;
    }
}

if (!$projectPath) {
    // Check explicit links first
    if (isset($links[$domain])) {
        $projectPath = $links[$domain];
    } else {
        // Check parked paths
        foreach ($paths as $path) {
            $potentialPath = "$path/$domain";
            if (is_dir($potentialPath)) {
                $projectPath = $potentialPath;
                break;
            }
        }
    }
}

if (!$projectPath) {
    http_response_code(404);
    echo "SLD: Site $host not found.";
    exit;
}

// 3. Serve the project
$publicPath = "$projectPath/public";
$indexPath = "$publicPath/index.php";

// If public/index.php doesn't exist, try project root
if (!file_exists($indexPath)) {
    $publicPath = $projectPath;
    $indexPath = "$projectPath/index.php";
}

if (file_exists($indexPath)) {
    // Simulate web server environment
    $_SERVER['DOCUMENT_ROOT'] = $publicPath;
    $_SERVER['SCRIPT_FILENAME'] = $indexPath;
    $_SERVER['PHP_SELF'] = '/index.php'; // Simplified

    // Serve static files if requested
    $uri = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
    if ($uri !== '/' && file_exists("$publicPath$uri") && !is_dir("$publicPath$uri")) {
        return false; // Let PHP built-in serve it? No, we are in FPM.
        // In FPM we can't "return false". We must serve it.
        // Actually, Nginx config should handle static files if possible.
        // But our generic rule sends everything to router.php.
        // We will just readfile() it with correct mime type for now.
        $mime = mime_content_type("$publicPath$uri");
        header("Content-Type: $mime");
        readfile("$publicPath$uri");
        exit;
    }

    chdir($publicPath);
    require $indexPath;
} else {
    // maybe it's just a static directory
    if (file_exists("$projectPath/index.html")) {
        readfile("$projectPath/index.html");
    } else {
        echo "SLD: project found at $projectPath but no index.php or index.html.";
    }
}
