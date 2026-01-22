const os = require('os');
const path = require('path');
const fs = require('fs');
const { spawn } = require('child_process');

const boreArgs = process.argv.slice(2);
const binDir = path.join(__dirname, "bin");
const binName = os.platform() === "win32" ? "bore.exe" : "bore";
const borePath = path.join(binDir, binName);

if (!fs.existsSync(borePath)) {
    console.error("❌ bore binary not found. Try reinstalling:");
    console.error("   npm install -g bore-cli");
    process.exit(1);
}

const boreProcess = spawn(borePath, boreArgs, { stdio: 'inherit' });

boreProcess.on('close', (code) => {
    process.exit(code);
});

boreProcess.on('error', (err) => {
    console.log(err);
    process.exit(1);
});
