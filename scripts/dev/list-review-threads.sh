#!/bin/bash
set -euo pipefail

# List GitHub Pull Request Review Threads
#
# This script shows unresolved review threads that may need replies.

show_usage() {
    echo "Usage: $0 <pr_number>"
    echo ""
    echo "Examples:"
    echo "  $0 287"
    echo ""
    echo "This will show:"
    echo "  - Unresolved review threads"
    echo "  - Thread IDs for use with review-reply.sh"
    echo "  - File locations and latest comments"
}

if [ $# -ne 1 ]; then
    show_usage
    exit 1
fi

PR_NUMBER="$1"

echo "🔍 Fetching review threads for PR #$PR_NUMBER..."

# Get review threads with enhanced information
gh api graphql -f query="
{
  repository(owner: \"apstndb\", name: \"spanner-mycli\") {
    pullRequest(number: $PR_NUMBER) {
      reviewThreads(first: 20) {
        nodes {
          id
          line
          path
          isResolved
          subjectType
          comments(first: 10) {
            nodes {
              id
              body
              author {
                login
              }
              createdAt
            }
          }
        }
      }
    }
  }
}" | jq -r '
  .data.repository.pullRequest.reviewThreads.nodes[] |
  select(.isResolved == false) |
  {
    "thread_id": .id,
    "location": "\(.path):\(.line // "file")",
    "subject_type": .subjectType,
    "latest_comment": (.comments.nodes[-1].body | .[0:100]),
    "latest_author": .comments.nodes[-1].author.login,
    "needs_reply": (.comments.nodes | map(.author.login) | unique | contains(["apstndb"]) | not)
  }
' | jq -s '
  if length == 0 then
    "✅ No unresolved review threads found"
  else
    "📋 Found \(length) unresolved review thread(s):",
    "",
    (.[] | "Thread ID: \(.thread_id)", "Location: \(.location)", "Author: \(.latest_author)", "Preview: \(.latest_comment)", "Needs Reply: \(.needs_reply)", "")
  end
'