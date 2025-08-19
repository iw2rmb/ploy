#!/bin/bash

# Fix PATH issue for ploy user on target host
# Run this on the target host as root to fix the PATH configuration

echo "Fixing PATH configuration for ploy user..."

# Fix .bashrc for ploy user
cat >> /home/ploy/.bashrc << 'EOF'

# Fix PATH to include basic system directories
export PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
EOF

# Update the setup-env.sh script if it exists
if [ -f /home/ploy/setup-env.sh ]; then
    echo "Updating setup-env.sh..."
    sed -i 's|export PATH="/usr/local/go/bin:$PATH"|export PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"|' /home/ploy/setup-env.sh
fi

# Set ownership
chown ploy:ploy /home/ploy/.bashrc
if [ -f /home/ploy/setup-env.sh ]; then
    chown ploy:ploy /home/ploy/setup-env.sh
fi

echo "PATH fix applied successfully!"
echo ""
echo "To apply the fix immediately, run as ploy user:"
echo "  su - ploy"
echo "  source ~/.bashrc"
echo "  # OR source ~/setup-env.sh"
echo ""
echo "The PATH should now include: /usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
