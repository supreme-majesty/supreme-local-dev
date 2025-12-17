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

// Check for phpmyadmin subdomain
if ($domain === 'phpmyadmin') {
     $possiblePaths = [
        '/usr/share/phpmyadmin',
        '/opt/lampp/phpmyadmin',
    ];

    foreach ($possiblePaths as $p) {
        if (is_dir($p)) {
            $projectPath = $p;
            break;
        }
    }
    
    if (!$projectPath) {
        http_response_code(404);
        echo "<h1>phpMyAdmin Not Found</h1>";
        echo "<p>Checked paths:</p><ul>";
        foreach ($possiblePaths as $p) echo "<li>$p</li>";
        echo "</ul>";
        exit;
    }
}

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
             // For localhost/phpmyadmin, we might need to strip the prefix for some router logic if we were doing internal routing,
            // but since we just set projectPath, the static file handler below needs to know the true URI relative to document root.
            // But wait, if we visit localhost/phpmyadmin/foo.css, the URI is /phpmyadmin/foo.css
            // But the file is at $toolPath/foo.css.
            // So we need to strip /phpmyadmin from the URI for file lookups further down?
            // Actually, the current router logic uses $publicPath . $uri
            // If projectPath is /opt/lampp/phpmyadmin, publicPath becomes that.
            // URI is /phpmyadmin/style.css.
            // File check: /opt/lampp/phpmyadmin/phpmyadmin/style.css -> FAIL.
            
            // We need to rewrite $_SERVER['REQUEST_URI'] or handle legacy localhost tool path stripping.
            // Simplest way: if we are here, we are committed to this tool.
            // Let's strip the prefix from URI for subsequent logic.
             $_SERVER['REQUEST_URI'] = substr($uri, strlen('/phpmyadmin'));
             if ($_SERVER['REQUEST_URI'] == '') $_SERVER['REQUEST_URI'] = '/';
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
    $targetFile = "$publicPath$uri";

    if ($uri !== '/' && file_exists($targetFile) && !is_dir($targetFile)) {
        $ext = pathinfo($targetFile, PATHINFO_EXTENSION);
        $mime = '';
        
        switch ($ext) {
            case 'css': $mime = 'text/css'; break;
            case 'js':  $mime = 'application/javascript'; break;
            case 'svg': $mime = 'image/svg+xml'; break;
            case 'png': $mime = 'image/png'; break;
            case 'jpg': 
            case 'jpeg': $mime = 'image/jpeg'; break;
            case 'gif': $mime = 'image/gif'; break;
            case 'webp': $mime = 'image/webp'; break;
            case 'ico': $mime = 'image/x-icon'; break;
            default: $mime = mime_content_type($targetFile);
        }
        
        header("Content-Type: $mime");
        readfile($targetFile);
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
