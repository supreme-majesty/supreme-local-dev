# Supreme Local Dev (SLD) 🚀

High-performance local development environment for PHP/Laravel, built with Go.

SLD provides a seamless experience for managing local web projects on Linux, offering automatic Nginx configuration, SSL management, and easy PHP version switching.

## Features

- **Project Parking**: Serve any directory of projects instantly.
- **Link System**: Create custom domains (e.g., `project.test`) for any path.
- **Automatic SSL**: Native HTTPS support via `mkcert`.
- **PHP Version Manager**: Switch between PHP versions (8.0, 8.1, 8.2, etc.) command.
- **Tools Integration**: Built-in support for phpMyAdmin.
- **GUI Dashboard**: Visual management of your sites and configurations.

## Installation

### Prerequisites

- Linux (Ubuntu/Debian recommended)
- Go 1.25+ (for building)

### Build & Install

```bash
# Clone the repository
git clone https://github.com/supreme-majesty/supreme-local-dev.git
cd supreme-local-dev

# Build the binary
go build -o sld cmd/sld/main.go

# Install binary (optional, or just add to PATH)
sudo mv sld /usr/local/bin/

# Initialize System Dependencies
sudo sld install
```

## Usage

### Basic Commands

- **`sld install`**: Install system dependencies (Nginx, PHP, Dnsmasq, etc.) and configure the environment.
- **`sld park [path]`**: Register a directory. All subdirectories will be served as `http://<dirname>.test`.
- **`sld link [name]`**: Link the current directory to `http://<name>.test`.
- **`sld secure`**: Generate SSL certificates and enable HTTPS for all `.test` domains.
- **`sld gui`**: Open the web-based dashboard.

### Managing Sites

```bash
# Park your projects folder
sld park ~/Developments

# Link a specific project
cd ~/dev/my-project
sld link my-project

# Remove a link
sld unlink my-project

# List all sites
sld paths   # Parked paths
sld links   # Linked sites
```

### PHP Management

Switch the global PHP version used by FPM:

```bash
sld php 8.2
sld php 8.1
```

### Services

```bash
# Check status of Nginx and PHP
sld status

# Start the background daemon (API)
sld daemon
```

## phpMyAdmin

Access phpMyAdmin at:

- http://phpmyadmin.test (Preferred)
- http://localhost/phpmyadmin

## License

MIT
