#!/usr/bin/env node

const os = require("os");
const fs = require("fs");
const path = require("path");
const https = require("https");
const tar = require("tar");

const { version, } = require("./package.json");
const platform = os.platform(); // 'aix', 'darwin', 'freebsd', 'linux', 'openbsd', 'sunos', 'win32'
const arch = os.arch(); // 'arm', 'arm64', 'ia32', 'loong64', 'mips', 'mipsel', 'ppc64', 'riscv64', 's390x', 'x64'

const supportedPlatformNames = {
    linux: "linux",
    darwin: "darwin",
    win32: "windows"
};

const supportedArchNames = {
    x64: "amd64",
    arm64: "arm64",
    arm: "armv6",
    ia32: "386"
};

const platfomName = supportedPlatformNames[platform];
const archName = supportedArchNames[arch];

if (!platfomName) {
    console.error(`❌ Unsupported platform: ${platform}`);
    process.exit(1);
}

if (!archName) {
    console.error(`❌ Unsupported architecture: ${platform} ${arch}`);
    process.exit(1);
}

const tag = `v${version}`;
const artifactName = `bore_${version}_${platfomName}_${archName}.tar.gz`;
const artifactURL = `https://github.com/Aditya-ds-1806/bore/releases/download/${tag}/${artifactName}`;

const binDir = path.join(__dirname, "bin");

fs.mkdirSync(binDir, { recursive: true });

console.log(`📦 Downloading bore (${artifactURL})...`);

function downloadArtifact(downloadURL, redirectCount = 5) {
    if (redirectCount === 0) {
        console.error("❌ Too many redirects");
        process.exit(1);
    }

    https.get(downloadURL, (res) => {
        if (res.statusCode === 302 || res.statusCode === 301) {
            downloadArtifact(res.headers.location, redirectCount - 1);
            res.destroy();
            return;
        }
        
        if (res.statusCode !== 200) {
            console.error(`❌ Failed to download: ${res.statusCode}`);
            process.exit(1);
        }
        
        const file = fs.createWriteStream(path.join(binDir, artifactName), { mode: 0o755 });
        res.pipe(file, { end: true });

        file.on("finish", () => {
            file.close((err) => {
                if (err) {
                    console.error(`❌ Error: ${err.message}`);
                    process.exit(1);
                }

                console.log(`✅ Downloaded to ${path.join(binDir, artifactName)}`);
                
                tar.x({
                    file: path.join(binDir, artifactName),
                    cwd: binDir,
                }).then(() => {
                    fs.unlinkSync(path.join(binDir, artifactName));
                    fs.unlinkSync(path.join(binDir, "bore-server"));
                    fs.chmodSync(path.join(binDir, "bore"), 0o755);
                    console.log("🎉 Installation complete!");
                }).catch((err) => {
                    console.error(`❌ Error extracting archive: ${err.message}`);
                    process.exit(1);
                });
            });
        });
    }).on("error", (err) => {
        console.error(`❌ Error: ${err.message}`);
        process.exit(1);
    });
}

downloadArtifact(artifactURL);
