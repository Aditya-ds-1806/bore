fetch('https://api.github.com/repos/Aditya-ds-1806/bore')
    .then(res => res.json())
    .then(data => {
        if (data.stargazers_count !== undefined) {
            const count = data.stargazers_count;
            const formatted = count >= 1000 ? (count / 1000).toFixed(1) + 'k' : count;
            document.getElementById('github-stars').textContent = formatted;
        }
    })
    .catch(() => { });

const header = document.querySelector('header');
window.addEventListener('scroll', () => {
    if (window.scrollY > 50) {
        header.classList.add('scrolled');
    } else {
        header.classList.remove('scrolled');
    }
});

const command = 'bore -u http://localhost:3000';
const typedCommand = document.getElementById('typed-command');
const typingCursor = document.getElementById('typing-cursor');
const terminalOutput = document.getElementById('terminal-output');
const finalPrompt = document.getElementById('final-prompt');

let charIndex = 0;

function resetTerminal() {
    charIndex = 0;
    typedCommand.textContent = '';
    typingCursor.style.display = 'inline-block';
    terminalOutput.style.display = 'none';
    finalPrompt.style.display = 'none';
}

function typeCommand() {
    if (charIndex < command.length) {
        typedCommand.textContent += command.charAt(charIndex);
        charIndex++;
        setTimeout(typeCommand, 50 + Math.random() * 50);
    } else {
        setTimeout(() => {
            typingCursor.style.display = 'none';
            terminalOutput.style.display = 'block';
            terminalOutput.style.animation = 'fadeIn 0.3s ease';
            setTimeout(() => {
                finalPrompt.style.display = 'flex';
                finalPrompt.style.animation = 'fadeIn 0.3s ease';

                setTimeout(() => {
                    resetTerminal();
                    setTimeout(typeCommand, 500);
                }, 3000);
            }, 300);
        }, 500);
    }
}

function renderInstallCommands() {
    const installBtns = document.querySelectorAll('.install-btn');
    const activeInstallBtn = document.querySelector('.install-btn.btn-primary');

    const installInstructions = {
        Homebrew: [
            'brew tap aditya-ds-1806/bore',
            'brew install bore --cask'
        ],
        Scoop: [
            'scoop bucket add aditya-ds-1806 https://github.com/aditya-ds-1806/scoop-bucket',
            'scoop install bore'
        ],
        Linux: ['curl -fsSL https://trybore.com/install.sh | sh'],
        NPM: ['Coming soon!']
    };

    installBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            installBtns.forEach(b => {
                b.classList.remove('btn-primary');
                b.classList.remove('btn-secondary');
            });

            btn.classList.add('btn-primary');

            const platform = btn.textContent.trim();
            const commands = installInstructions[platform];
            
            const stepCode = document.querySelector('.install-commands-container .step-code');
            stepCode.innerHTML = commands.join('<br>');
        });
    });
}

renderInstallCommands();

setTimeout(typeCommand, 1000);
