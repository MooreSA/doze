#!/bin/bash
# Run the API server with sprites enabled (loads .env file)

set -a  # Export all variables
source .env
set +a

cd api
echo "🚀 Starting Doze API with Sprites..."
echo "   SPRITES_API_KEY: ${SPRITES_API_KEY:0:20}..."
echo "   SPRITES_CHECKPOINT: $SPRITES_CHECKPOINT"
echo "   REPO_URL: $REPO_URL"
echo "   PORT: $PORT"
echo ""

./doze-api
