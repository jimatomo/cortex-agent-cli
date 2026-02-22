# 公式 Go イメージ (Debian Bookworm) をベースにする
FROM golang:1.24-bookworm

# 共通依存と vscode ユーザー（devcontainer 用）
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    zsh \
    sudo \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r vscode -g 1000 \
    && useradd -r -u 1000 -g vscode -m -s /bin/bash vscode \
    && echo "vscode ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Graphite CLI (npm)
# RUN npm install -g @withgraphite/graphite-cli@stable

# vscode ユーザーで Oh My Zsh とプラグインをセットアップ
USER vscode
RUN sh -c "$(curl -fsSL https://raw.github.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended \
    && mkdir -p "$HOME/.oh-my-zsh/custom/plugins" \
    && git clone https://github.com/zsh-users/zsh-completions "$HOME/.oh-my-zsh/custom/plugins/zsh-completions" \
    && git clone https://github.com/zsh-users/zsh-autosuggestions "$HOME/.oh-my-zsh/custom/plugins/zsh-autosuggestions" \
    && sed -i -e 's/^plugins=(.*)/plugins=(git zsh-completions zsh-autosuggestions)/' "$HOME/.zshrc"

# Claude Code
RUN curl -fsSL https://claude.ai/install.sh | bash
RUN echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.zshrc"
