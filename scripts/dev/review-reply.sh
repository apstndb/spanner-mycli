#!/bin/bash
set -euo pipefail

# GitHub Review Thread Reply Helper
# 
# This script helps reply to GitHub pull request review threads using GraphQL mutations.
#
# IMPORTANT DISCOVERY: pullRequestReviewId field causes failures when included.
# The GitHub GraphQL schema shows this field as optional, but including it often
# results in null responses. Always omit it for simple thread replies.

show_usage() {
    echo "Usage: $0 <thread_id> <reply_text> [mention_user]"
    echo ""
    echo "Examples:"
    echo "  $0 PRRT_kwDONC6gMM5SU-GH \"Fixed the issue as suggested\""
    echo "  $0 PRRT_kwDONC6gMM5SU-GH \"Thank you for the feedback!\" gemini-code-assist"
    echo ""
    echo "Arguments:"
    echo "  thread_id    - GitHub review thread ID (from GraphQL query)"
    echo "  reply_text   - Text content of the reply"
    echo "  mention_user - Optional: username to mention (without @)"
    echo ""
    echo "To get thread IDs, use:"
    echo "  scripts/dev/list-review-threads.sh <pr_number>"
}

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
    show_usage
    exit 1
fi

THREAD_ID="$1"
REPLY_TEXT="$2"
MENTION_USER="${3:-}"

# Add mention if provided
if [ -n "$MENTION_USER" ]; then
    REPLY_TEXT="@$MENTION_USER $REPLY_TEXT"
fi

echo "🔄 Replying to review thread: $THREAD_ID"

# Create GraphQL mutation file
# CRITICAL: Do NOT include pullRequestReviewId - it causes null responses
cat > /tmp/review_reply.graphql << EOF
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "$THREAD_ID"
    body: "$REPLY_TEXT"
  }) {
    comment {
      id
      url
      body
    }
  }
}
EOF

# Execute the mutation
echo "📤 Sending reply..."
RESULT=$(gh api graphql -F query=@/tmp/review_reply.graphql)

# Check if the reply was successful
COMMENT_ID=$(echo "$RESULT" | jq -r '.data.addPullRequestReviewThreadReply.comment.id // empty')
COMMENT_URL=$(echo "$RESULT" | jq -r '.data.addPullRequestReviewThreadReply.comment.url // empty')

if [ -n "$COMMENT_ID" ]; then
    echo "✅ Reply posted successfully!"
    echo "   Comment ID: $COMMENT_ID"
    echo "   URL: $COMMENT_URL"
else
    echo "❌ Failed to post reply. Response:"
    echo "$RESULT" | jq '.'
    exit 1
fi

# Cleanup
rm -f /tmp/review_reply.graphql