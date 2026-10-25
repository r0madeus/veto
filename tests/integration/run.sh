#!/bin/bash

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

echo "🚀 Starting Integration Tests..."

# Initialize Veto (Create dirs)
veto init --yes

# ==========================================
# Scenario 1: Happy Path (Install 'nano')
# ==========================================
echo -n "Test 1: Installing 'nano' (Happy Path)... "

# Ensure nano is NOT installed (it comes with base-devel but let's remove it first to be sure)
pacman -Rns --noconfirm nano > /dev/null 2>&1 || true

# Create config
cat <<EOF > success.yaml
resources:
  - id: install-nano
    type: pkg
    name: nano
    state: present
EOF

# Apply
OUTPUT=$(veto apply success.yaml 2>&1)
echo "$OUTPUT"

if echo "$OUTPUT" | grep -q "Successfully present package nano"; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# Verify installation
if ! pacman -Qi nano > /dev/null 2>&1; then
    echo -e "${RED}FAIL (Package not found)${NC}"
    exit 1
fi

# ==========================================
# Scenario 2: Idempotency (Run again)
# ==========================================
echo -n "Test 2: Idempotency (Run again)... "

# The "no change" message is logged at debug level (-vv); logs go to stderr.
OUTPUT=$(veto -vv apply success.yaml 2>&1)
if echo "$OUTPUT" | grep -q "already in desired state"; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}"
    exit 1
fi

# ==========================================
# Scenario 3: Identity Management (Create User)
# ==========================================
echo -n "Test 3: Identity Management (Create User)... "

# Ensure user doesn't exist
userdel -r veto_test > /dev/null 2>&1 || true

cat <<EOF > user.yaml
resources:
  - id: create-user
    type: user
    name: veto_test
    params:
      uid: "2000"
      shell: /bin/sh
      home: /home/veto_test
    state: present
EOF

OUTPUT=$(veto apply user.yaml 2>&1)
if echo "$OUTPUT" | grep -q "User created"; then
    # Verify with system command
    if id veto_test > /dev/null 2>&1; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL (System verification failed)${NC}"
        exit 1
    fi
else
    echo -e "${RED}FAIL (Apply failed)${NC}"
    exit 1
fi

# ==========================================
# Scenario 4: User Rollback (Atomic Failure)
# ==========================================
echo -n "Test 4: User Rollback... "

# Remove user again to have clean slate for rollback test
userdel -r veto_test > /dev/null 2>&1 || true

cat <<EOF > user_fail.yaml
resources:
  - id: create-user-rollback
    type: user
    name: veto_test
    params:
      uid: "2001"
    state: present
  - id: fail-step
    type: pkg
    name: missing-package-xyz
    state: present
EOF

# Run Apply (Should fail)
set +e
veto apply user_fail.yaml > /dev/null 2>&1
EXIT_CODE=$?
set -e

if [ $EXIT_CODE -eq 0 ]; then
     echo -e "${RED}FAIL (Should have failed)${NC}"
     exit 1
fi

if ! id veto_test > /dev/null 2>&1; then
    echo -e "${GREEN}PASS (User reverted)${NC}"
else
    echo -e "${RED}FAIL (User still exists)${NC}"
    exit 1
fi

# ==========================================
# Scenario 5: Package Rollback (Atomic Failure)
# ==========================================
echo -n "Test 5: Package Rollback... "

# Install 'vim' (valid) but fail on 'invalid-pkg'
# We remove vim first
pacman -Rns --noconfirm vim > /dev/null 2>&1 || true

cat <<EOF > fail.yaml
resources:
  - id: install-vim
    type: pkg
    name: vim
    state: present
  - id: install-invalid
    type: pkg
    name: this-package-does-not-exist-12345
    state: present
EOF

# Run Apply (Should fail)
set +e # Disable exit on error temporarily
OUTPUT=$(veto apply fail.yaml 2>&1)
EXIT_CODE=$?
set -e

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${RED}FAIL (Should have failed)${NC}"
    exit 1
fi

# Verify Rollback (Vim should be gone)
if pacman -Qi vim > /dev/null 2>&1; then
    echo -e "${RED}FAIL (Rollback failed - vim is still installed)${NC}"
    exit 1
else
    echo -e "${GREEN}PASS (Vim correctly reverted)${NC}"
fi

echo ""
echo -e "✅ All Integration Tests Passed!"
