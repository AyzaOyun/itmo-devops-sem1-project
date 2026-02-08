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
    "time"

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
    connStr := "user=validator password=val1dat0r dbname=project-sem-1 host=localhost port=5432 sslmode=disable"
    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    defer db.Close()

    // Ждем подключения к БД
    for i := 0; i < 10; i++ {
        if err := db.Ping(); err != nil {
            log.Printf("Attempt %d: Database ping failed, retrying...", i+1)
            time.Sleep(2 * time.Second)
        } else {
            break
        }
    }
    
    if err := db.Ping(); err != nil {
        log.Fatal("Database ping failed after retries:", err)
    }
    log.Println("Connected to PostgreSQL database")

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
        name TEXT NOT NULL,
        category TEXT NOT NULL,
        price DECIMAL(10, 2) NOT NULL,
        create_date TIMESTAMP NOT NULL
    )`
    _, err := db.Exec(query)
    if err != nil {
        log.Printf("Table creation warning: %v", err)
    } else {
        log.Println("Prices table ready")
    }
}

func handlePrices(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        handlePost(w, r)
    case http.MethodGet:
        handleGet(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func handlePost(w http.ResponseWriter, r *http.Request) {
    log.Println("POST /api/v0/prices request received")

    // Поддержка multipart/form-data
    if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
        err := r.ParseMultipartForm(10 << 20)
        if err != nil {
            http.Error(w, "Failed to parse form", http.StatusBadRequest)
            return
        }
        
        file, _, err := r.FormFile("file")
        if err != nil {
            http.Error(w, "File not found", http.StatusBadRequest)
            return
        }
        defer file.Close()
        processCSV(file, w)
        return
    }
    
    // Оригинальная логика для application/zip
    processCSV(r.Body, w)
}

func processCSV(reader io.Reader, w http.ResponseWriter) {
    body, err := io.ReadAll(reader)
    if err != nil {
        http.Error(w, "Failed to read data", http.StatusBadRequest)
        return
    }

    if len(body) == 0 {
        http.Error(w, "Empty data", http.StatusBadRequest)
        return
    }

    zipReader, err := zip.NewReader(strings.NewReader(string(body)), int64(len(body)))
    if err != nil {
        http.Error(w, "Invalid ZIP", http.StatusBadRequest)
        return
    }
    
    var csvFile *zip.File
    for _, f := range zipReader.File {
        baseName := filepath.Base(f.Name)
        if baseName == "data.csv" || baseName == "test_data.csv" {
            csvFile = f
            break
        }
    }
    if csvFile == nil {
        http.Error(w, "CSV file not found", http.StatusBadRequest)
        return
    }

    rc, err := csvFile.Open()
    if err != nil {
        http.Error(w, "Failed to open CSV", http.StatusInternalServerError)
        return
    }
    defer rc.Close()

    csvReader := csv.NewReader(rc)
    records, err := csvReader.ReadAll()
    if err != nil {
        http.Error(w, "Failed to read CSV", http.StatusInternalServerError)
        return
    }

    if len(records) == 0 {
        http.Error(w, "Empty CSV", http.StatusBadRequest)
        return
    }

    stats := Stats{}
    categorySet := make(map[string]bool)

    tx, err := db.Begin()
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO prices (name, category, price, create_date) 
        VALUES ($1, $2, $3, $4)
    `)
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer stmt.Close()
    
    // Пропускаем заголовок если он есть
    start := 0
    if len(records) > 0 {
        firstRecord := strings.ToLower(strings.Join(records[0], ","))
        if strings.Contains(firstRecord, "id") && 
           strings.Contains(firstRecord, "create_date") && 
           strings.Contains(firstRecord, "name") && 
           strings.Contains(firstRecord, "category") && 
           strings.Contains(firstRecord, "price") {
            start = 1
        }
    }
    
    // ВАЖНО: формат CSV согласно заданию:
    // id, create_date, name, category, price
    for i := start; i < len(records); i++ {
        record := records[i]
        if len(record) < 5 {
            continue
        }

        // Парсим поля в правильном порядке:
        // record[0] - id (не используем, т.к. SERIAL в БД)
        dateStr := strings.TrimSpace(record[1])  // create_date
        name := strings.TrimSpace(record[2])     // name
        category := strings.TrimSpace(record[3]) // category
        priceStr := strings.TrimSpace(record[4]) // price

        // Пропускаем пустые строки
        if name == "" || category == "" || priceStr == "" || dateStr == "" {
            continue
        }

        price, err := strconv.ParseFloat(priceStr, 64)
        if err != nil {
            continue
        }

        // Парсим дату (формат: ГОД–МЕСЯЦ–ДЕНЬ = YYYY-MM-DD)
        createDate, err := time.Parse("2006-01-02", dateStr)
        if err != nil {
            continue
        }

        _, err = stmt.Exec(name, category, price, createDate)
        if err != nil {
            continue
        }
    
        stats.TotalItems++
        categorySet[category] = true
        stats.TotalPrice += price
    }
    
    if err := tx.Commit(); err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    stats.TotalCategories = len(categorySet)

    log.Printf("Inserted: %d items, %d categories, total price: %.2f", 
        stats.TotalItems, stats.TotalCategories, stats.TotalPrice)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(stats)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
    log.Println("GET /api/v0/prices request received")

    // ВАЖНО: правильный порядок полей согласно заданию:
    // id, create_date, name, category, price
    rows, err := db.Query(`
        SELECT id, create_date, name, category, price 
        FROM prices 
        ORDER BY id
    `)
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    csvFile, err := os.CreateTemp("", "data-*.csv")
    if err != nil {
        http.Error(w, "Failed to create CSV file", http.StatusInternalServerError)
        return
    }
    defer os.Remove(csvFile.Name())
    defer csvFile.Close()

    csvWriter := csv.NewWriter(csvFile)
    
    // ВАЖНО: заголовок CSV в правильном порядке согласно заданию:
    csvWriter.Write([]string{"id", "create_date", "name", "category", "price"})
    
    for rows.Next() {
        var id int
        var createDate time.Time
        var name, category string
        var price float64
        
        if err := rows.Scan(&id, &createDate, &name, &category, &price); err != nil {
            continue
        }
        
        // Формируем запись в правильном порядке:
        record := []string{
            strconv.Itoa(id),
            createDate.Format("2006-01-02"), // ГОД–МЕСЯЦ–ДЕНЬ
            name,
            category,
            strconv.FormatFloat(price, 'f', 2, 64),
        }
        
        csvWriter.Write(record)
    }
    
    // Проверяем ошибки после цикла
    if err = rows.Err(); err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
        
    csvWriter.Flush()
    if err := csvWriter.Error(); err != nil {
        http.Error(w, "Failed to write CSV", http.StatusInternalServerError)
        return
    }

    csvFile.Seek(0, 0)

    zipFile, err := os.CreateTemp("", "data-*.zip")
    if err != nil {
        http.Error(w, "Failed to create ZIP file", http.StatusInternalServerError)
        return
    }
    defer os.Remove(zipFile.Name())
    defer zipFile.Close()

    zipWriter := zip.NewWriter(zipFile)
    
    fileInZip, err := zipWriter.Create("data.csv")
    if err != nil {
        http.Error(w, "Failed to create file in ZIP", http.StatusInternalServerError)
        return
    }

    io.Copy(fileInZip, csvFile)
    zipWriter.Close()
    zipFile.Seek(0, 0)

    w.Header().Set("Content-Type", "application/zip")
    w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
    io.Copy(w, zipFile)
    
    log.Println("ZIP archive sent successfully")
}
