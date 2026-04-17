#!/bin/bash
# Attack station entrypoint
# Ensures team users exist with the documented password AND password SSH is enabled
# so the range is competition-standard every time the container starts.

set -e

ssh-keygen -A 2>/dev/null

# --- Enable password SSH (competition standard) ---
sed -i 's/^#\?PasswordAuthentication .*/PasswordAuthentication yes/' /etc/ssh/sshd_config
grep -q '^PasswordAuthentication yes' /etc/ssh/sshd_config || \
    echo 'PasswordAuthentication yes' >> /etc/ssh/sshd_config
sed -i 's/^#\?PermitRootLogin .*/PermitRootLogin no/' /etc/ssh/sshd_config
grep -q '^UsePAM yes' /etc/ssh/sshd_config || echo 'UsePAM yes' >> /etc/ssh/sshd_config

# --- Create default team users with documented password ---
# The default below is the value the original team used during practice
# so the competition-parity docs stay correct out of the box. OPERATORS
# MUST override TEAM_PASS via the container environment before sharing a
# deployment — see SECURITY.md in the repo root.
TEAM_PASS="${TEAM_PASS:-phoenix123}"
for user in dash laura phoenix; do
    if ! id "$user" &>/dev/null; then
        adduser -D -G team -s /bin/bash "$user" 2>/dev/null || \
            useradd -m -G team -s /bin/bash "$user"
    fi
    echo "${user}:${TEAM_PASS}" | chpasswd
    mkdir -p "/home/$user"
    chown "$user:team" "/home/$user" 2>/dev/null || true
done

# --- Optional: also accept SSH keys from /keys volume for users who provide them ---
if [ -d /keys ]; then
    for keyfile in /keys/*.pub; do
        if [ -f "$keyfile" ]; then
            username=$(basename "$keyfile" .pub)
            if ! id "$username" &>/dev/null; then
                adduser -D -G team -s /bin/bash "$username" 2>/dev/null || \
                    useradd -m -G team -s /bin/bash "$username"
                echo "${username}:${TEAM_PASS}" | chpasswd
            fi
            mkdir -p "/home/$username/.ssh"
            cp "$keyfile" "/home/$username/.ssh/authorized_keys"
            chmod 700 "/home/$username/.ssh"
            chmod 600 "/home/$username/.ssh/authorized_keys"
            chown -R "$username:team" "/home/$username"
        fi
    done
fi

chmod 1777 /workspace 2>/dev/null || true

# --- Grant team access to Docker socket (needed for reset-range) ---
if [ -S /var/run/docker.sock ]; then
    chmod 666 /var/run/docker.sock
fi

# Write MOTD
cat > /etc/motd << 'MOTD'

=== GILDED GUARDIAN — ATTACK STATION ===

Your attack station is ready. Good luck.

MOTD

exec /usr/sbin/sshd -D -e
