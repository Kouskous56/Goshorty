#!/bin/bash
# GoShorty API Testing Script

API_URL="http://localhost:8080"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}GoShorty API Testing${NC}\n"

# Test 1: Health Check
echo -e "${GREEN}1. Health Check${NC}"
curl -s "$API_URL/health" | jq '.'
echo ""

# Test 2: Create Short URL
echo -e "${GREEN}2. Create Short URL${NC}"
SHORTEN_RESPONSE=$(curl -s -X POST "$API_URL/api/shorten" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://github.com/torvalds/linux",
    "expires_in": "24h"
  }')
echo "$SHORTEN_RESPONSE" | jq '.'
SHORT_CODE=$(echo "$SHORTEN_RESPONSE" | jq -r '.short_code')
echo ""

# Test 3: Create with Custom Code
echo -e "${GREEN}3. Create with Custom Code${NC}"
CUSTOM_RESPONSE=$(curl -s -X POST "$API_URL/api/shorten" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://golang.org",
    "expires_in": "1h",
    "custom_code": "go"
  }')
echo "$CUSTOM_RESPONSE" | jq '.'
echo ""

# Test 4: Get URL Info
echo -e "${GREEN}4. Get URL Info${NC}"
curl -s "$API_URL/api/shorten/$SHORT_CODE" | jq '.'
echo ""

# Test 5: Statistics
echo -e "${GREEN}5. Get Statistics${NC}"
curl -s "$API_URL/api/stats" | jq '.'
echo ""

# Test 6: Create 5-minute TTL
echo -e "${GREEN}6. Create 5-minute TTL${NC}"
curl -s -X POST "$API_URL/api/shorten" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "expires_in": "5m"
  }' | jq '.'
echo ""

# Test 7: List All URLs
echo -e "${GREEN}7. List All URLs${NC}"
curl -s "$API_URL/api/shorten/all" | jq '.urls | length'
echo ""

echo -e "${BLUE}Testing complete!${NC}"
