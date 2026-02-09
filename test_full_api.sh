#!/bin/bash

echo "=== Full API Test ==="

# 1. Create test data
echo "1. Creating test data..."
cat > test_api.csv << CSV
id,name,category,price,create_date
1,Product1,Category1,100.50,2024-01-01
2,Product2,Category2,200.75,2024-01-02
3,Product3,Category1,150.00,2024-01-03
CSV
zip test_api.zip test_api.csv

# 2. Test POST
echo -e "\n2. Testing POST /api/v0/prices..."
POST_RESPONSE=$(curl -s -F "file=@test_api.zip" http://localhost:8080/api/v0/prices)
echo "POST Response: $POST_RESPONSE"

# Check POST response format
if [[ $POST_RESPONSE == *"total_items"* && $POST_RESPONSE == *"total_categories"* && $POST_RESPONSE == *"total_price"* ]]; then
    echo "✓ POST response contains required fields"
else
    echo "✗ POST response missing required fields"
fi

# 3. Test GET
echo -e "\n3. Testing GET /api/v0/prices..."
curl -s http://localhost:8080/api/v0/prices -o get_test.zip

if [ -f get_test.zip ]; then
    echo "✓ GET request successful (got ZIP file)"
    
    # Check ZIP contents
    if unzip -l get_test.zip 2>/dev/null | grep -q "data.csv"; then
        echo "✓ ZIP contains data.csv"
        
        # Extract and show first few lines
        unzip -p get_test.zip data.csv | head -5 > /tmp/extracted.csv
        echo "First lines of data.csv:"
        cat /tmp/extracted.csv
    else
        echo "✗ ZIP doesn't contain data.csv"
        unzip -l get_test.zip
    fi
else
    echo "✗ GET request failed"
fi

# 4. Cleanup
echo -e "\n4. Cleanup..."
rm -f test_api.csv test_api.zip get_test.zip /tmp/extracted.csv 2>/dev/null

echo -e "\n=== Test complete ==="
