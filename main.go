package main

import (
    "archive/zip"
    "database/sql"
    "encoding/csv"
    "encoding/json"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"

    "github.com/gorilla/mux"
    _ "github.com/lib/pq"
)

type Stats struct {
    TotalItems      int     `json:"total_items"`
    TotalCategories int     `json:"total_categories"`
    TotalPrice      float64 `json:"total_price"`
}

var db *sql.DB

func main() {
    connStr := "user=validator password=validator dbname=project-sem-1 host=localhost port=5432 sslmode=disable"
    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    if err := db.Ping(); err != nil {
        log.Fatal(err)
    }

    createTable()

    r := mux.NewRouter()
    r.HandleFunc("/api/v0/prices", handlePrices).Methods("POST", "GET")

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", r))
}

func createTable() {
    query := `
    CREATE TABLE IF NOT EXISTS prices (
        id SERIAL PRIMARY KEY,
        product_id INTEGER,
        name TEXT,
        category TEXT,
        price DECIMAL(10, 2),
        create_date DATE
    )`
    _, err := db.Exec(query)
    if err != nil {
        log.Printf("warning: %v", err)
    }
}

func handlePrices(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        handlePost(w, r)
    } else if r.Method == http.MethodGet {
        handleGet(w, r)
    } else {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func handlePost(w http.ResponseWriter, r *http.Request) {
    log.Println("POST request")

    // read body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "failed to read body", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    if len(body) == 0 {
        http.Error(w, "empty body", http.StatusBadRequest)
        return
    }

    // open zip
    zipReader, err := zip.NewReader(strings.NewReader(string(body)), int64(len(body)))
    if err != nil {
        http.Error(w, "invalid zip", http.StatusBadRequest)
        return
    }

    // find csv
    var csvFile *zip.File
    for _, file := range zipReader.File {
        if filepath.Base(file.Name) == "data.csv" {
            csvFile = file
            break
        }
    }
    if csvFile == nil {
        http.Error(w, "data.csv not found", http.StatusBadRequest)
        return
    }

    // open csv
    rc, err := csvFile.Open()
    if err != nil {
        http.Error(w, "failed to open csv", http.StatusInternalServerError)
        return
    }
    defer rc.Close()

    // read csv
    csvReader := csv.NewReader(rc)
    records, err := csvReader.ReadAll()
    if err != nil {
        http.Error(w, "failed to read csv", http.StatusInternalServerError)
        return
    }

    if len(records) == 0 {
        http.Error(w, "empty csv", http.StatusBadRequest)
        return
    }

    stats := Stats{}
    categorySet := make(map[string]bool)

    tx, err := db.Begin()
    if err != nil {
        http.Error(w, "database error", http.StatusInternalServerError)
        return
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO prices (product_id, name, category, price, create_date) 
        VALUES ($1, $2, $3, $4, $5)
    `)
    if err != nil {
        http.Error(w, "database error", http.StatusInternalServerError)
        return
    }
    defer stmt.Close()

    // skip header if exists
    start := 0
    if len(records) > 0 && records[0][0] == "id" {
        start = 1
    }

    for i := start; i < len(records); i++ {
        record := records[i]
        if len(record) < 5 {
            continue
        }

        productID, _ := strconv.Atoi(strings.TrimSpace(record[0]))
        name := strings.TrimSpace(record[1])
        category := strings.TrimSpace(record[2])
        priceStr := strings.TrimSpace(record[3])
        createDate := strings.TrimSpace(record[4])

        price, err := strconv.ParseFloat(priceStr, 64)
        if err != nil {
            continue
        }

        _, err = stmt.Exec(productID, name, category, price, createDate)
        if err != nil {
            continue
        }

        stats.TotalItems++
        categorySet[category] = true
        stats.TotalPrice += price
    }

    if err := tx.Commit(); err != nil {
        http.Error(w, "database error", http.StatusInternalServerError)
        return
    }

    stats.TotalCategories = len(categorySet)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(stats)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
    log.Println("GET request")

    rows, err := db.Query(`
        SELECT product_id, name, category, price, create_date 
        FROM prices 
        ORDER BY id
    `)
    if err != nil {
        http.Error(w, "database error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    // create temp csv
    csvFile, err := os.CreateTemp("", "data-*.csv")
    if err != nil {
        http.Error(w, "failed to create csv", http.StatusInternalServerError)
        return
    }
    defer os.Remove(csvFile.Name())
    defer csvFile.Close()

    csvWriter := csv.NewWriter(csvFile)
    
    csvWriter.Write([]string{"id", "name", "category", "price", "create_date"})
    
    for rows.Next() {
        var productID int
        var name, category string
        var price float64
        var createDate string
        
        if err := rows.Scan(&productID, &name, &category, &price, &createDate); err != nil {
            continue
        }
        
        record := []string{
            strconv.Itoa(productID),
            name,
            category,
            strconv.FormatFloat(price, 'f', 2, 64),
            createDate,
        }
        
        csvWriter.Write(record)
    }
    
    csvWriter.Flush()
    if err := csvWriter.Error(); err != nil {
        http.Error(w, "failed to write csv", http.StatusInternalServerError)
        return
    }

    csvFile.Seek(0, 0)

    // create zip
    zipFile, err := os.CreateTemp("", "data-*.zip")
    if err != nil {
        http.Error(w, "failed to create zip", http.StatusInternalServerError)
        return
    }
    defer os.Remove(zipFile.Name())
    defer zipFile.Close()

    zipWriter := zip.NewWriter(zipFile)
    
    fileInZip, err := zipWriter.Create("data.csv")
    if err != nil {
        http.Error(w, "failed to create file in zip", http.StatusInternalServerError)
        return
    }

    io.Copy(fileInZip, csvFile)
    zipWriter.Close()
    zipFile.Seek(0, 0)

    w.Header().Set("Content-Type", "application/zip")
    w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
    io.Copy(w, zipFile)
}
