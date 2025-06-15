#!/bin/bash

set -euo pipefail

# Usage: ./check-pr-reviews.sh <PR_NUMBER> [OWNER] [REPO]
# Example: ./check-pr-reviews.sh 259 apstndb spanner-mycli

PR_NUMBER="${1:-}"
OWNER="${2:-apstndb}"
REPO="${3:-spanner-mycli}"

if [ -z "$PR_NUMBER" ]; then
    echo "Usage: $0 <PR_NUMBER> [OWNER] [REPO]"
    echo "Example: $0 259 apstndb spanner-mycli"
    exit 1
fi

# Configuration files for tracking state
STATE_DIR="${HOME}/.cache/spanner-mycli-reviews"
LAST_REVIEW_FILE="${STATE_DIR}/pr-${PR_NUMBER}-last-review.json"

mkdir -p "$STATE_DIR"

echo "Checking reviews for PR #${PR_NUMBER} in ${OWNER}/${REPO}..."

# Fetch latest reviews
LATEST_REVIEWS=$(gh api graphql -f query="
query {
  repository(owner: \"$OWNER\", name: \"$REPO\") {
    pullRequest(number: $PR_NUMBER) {
      reviews(last: 15) {
        nodes {
          id
          author {
            login
          }
          createdAt
          state
          body
        }
      }
    }
  }
}" | jq -r '.data.repository.pullRequest.reviews.nodes | sort_by(.createdAt) | reverse')

if [ "$LATEST_REVIEWS" = "null" ] || [ -z "$LATEST_REVIEWS" ]; then
    echo "Error: Could not fetch reviews or no reviews found"
    exit 1
fi

# Check if we have previous state
if [ -f "$LAST_REVIEW_FILE" ]; then
    echo "Found previous review state, checking for updates..."
    
    LAST_KNOWN_REVIEW_ID=$(jq -r '.id' "$LAST_REVIEW_FILE")
    LAST_TIMESTAMP=$(jq -r '.createdAt' "$LAST_REVIEW_FILE")
    
    echo "Last known review: $LAST_KNOWN_REVIEW_ID at $LAST_TIMESTAMP"
    
    # Check if previous review is still in the results (data integrity check)
    PREVIOUS_REVIEW_FOUND=$(echo "$LATEST_REVIEWS" | jq -r "map(select(.id == \"$LAST_KNOWN_REVIEW_ID\")) | length > 0")
    
    if [ "$PREVIOUS_REVIEW_FOUND" = "false" ]; then
        echo "Warning: Previous review not found in latest results. May need to fetch more reviews."
        echo "Proceeding with full review list..."
    else
        echo "✓ Previous review found, data integrity confirmed"
    fi
    
    # Find truly new reviews
    NEW_REVIEWS=$(echo "$LATEST_REVIEWS" | jq -r "map(select((.createdAt > \"$LAST_TIMESTAMP\") or (.createdAt == \"$LAST_TIMESTAMP\" and .id != \"$LAST_KNOWN_REVIEW_ID\"))) | sort_by(.createdAt)")
    
    NEW_COUNT=$(echo "$NEW_REVIEWS" | jq -r 'length')
    
    if [ "$NEW_COUNT" -gt 0 ]; then
        echo ""
        echo "🆕 Found $NEW_COUNT new review(s):"
        echo "$NEW_REVIEWS" | jq -r '.[] | "  - \(.author.login) at \(.createdAt) (\(.state))"'
        echo ""
        
        # Show detailed new reviews
        echo "📝 New review details:"
        echo "$NEW_REVIEWS" | jq -r '.[] | "
Author: \(.author.login)
Time: \(.createdAt)
State: \(.state)
Body: \(.body // "No body")
---"'
    else
        echo "✓ No new reviews since last check"
    fi
else
    echo "No previous state found, showing all recent reviews..."
    NEW_REVIEWS="$LATEST_REVIEWS"
    NEW_COUNT=$(echo "$NEW_REVIEWS" | jq -r 'length')
    
    echo ""
    echo "📋 Found $NEW_COUNT review(s) total:"
    echo "$NEW_REVIEWS" | jq -r '.[] | "  - \(.author.login) at \(.createdAt) (\(.state))"'
fi

# Update state with latest review
if [ "$NEW_COUNT" -gt 0 ] || [ ! -f "$LAST_REVIEW_FILE" ]; then
    LATEST_REVIEW=$(echo "$LATEST_REVIEWS" | jq -r '.[0]')
    echo "$LATEST_REVIEW" > "$LAST_REVIEW_FILE"
    
    LATEST_ID=$(echo "$LATEST_REVIEW" | jq -r '.id')
    LATEST_TIME=$(echo "$LATEST_REVIEW" | jq -r '.createdAt')
    
    echo ""
    echo "💾 Updated state: Latest review $LATEST_ID at $LATEST_TIME"
fi

echo ""
echo "✅ Review check complete"