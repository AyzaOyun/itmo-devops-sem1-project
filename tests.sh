#!/bin/bash
# quick tests

# post test
if [ -f "sample_data.zip" ]; then
    curl -s -X POST http://localhost:8080/api/v0/prices \
        -H "Content-Type: application/zip" \
        --data-binary "@sample_data.zip"
    echo ""
fi

# get test
curl -s -X GET http://localhost:8080/api/v0/prices -o test.zip

if [ -f "test.zip" ]; then
    echo "downloaded test.zip"
    unzip -l test.zip 2>/dev/null || echo "can't list zip"
fi
