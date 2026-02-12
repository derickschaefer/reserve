#!/usr/bin/env bash
# -----------------------------------------------------
# gitpush.sh
# Helper script to stage, commit, and push reserve
# Repo: https://github.com/derickschaefer/reserve
# -----------------------------------------------------

set -e

# Move to the directory of this script
cd "$(dirname "$0")"

# Ensure Git trusts this directory (useful when running as root)
git config --global --add safe.directory "$(pwd)" >/dev/null 2>&1 || true

# Ensure origin is set correctly
EXPECTED_REMOTE="git@github.com:derickschaefer/reserve.git"
CURRENT_REMOTE=$(git remote get-url origin 2>/dev/null || echo "")

if [ "$CURRENT_REMOTE" != "$EXPECTED_REMOTE" ]; then
  echo "Setting remote origin to $EXPECTED_REMOTE"
  git remote remove origin 2>/dev/null || true
  git remote add origin "$EXPECTED_REMOTE"
fi

# Determine current branch
CURRENT_BRANCH=$(git branch --show-current)

if [ -z "$CURRENT_BRANCH" ]; then
  echo "Unable to determine current branch."
  exit 1
fi

echo "Branch: $CURRENT_BRANCH"
echo

# Stage all changes
git add .

# Check if anything is staged
if git diff --cached --quiet; then
  echo "No changes to commit."
  exit 0
fi

# Ask for commit message
read -rp "Enter commit message: " COMMIT_MSG

# Default message if none entered
if [ -z "$COMMIT_MSG" ]; then
  COMMIT_MSG="reserve: update $(date +'%Y-%m-%d %H:%M:%S')"
fi

# Commit
git commit -m "$COMMIT_MSG"

# Push
git push -u origin "$CURRENT_BRANCH"

echo
echo "Successfully pushed to GitHub (branch: $CURRENT_BRANCH)"
