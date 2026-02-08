package main

import (
    "archive/zip"
    "database/sql"
    "encoding/csv"
    "encoding/json"
    "fmt"
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
    // Используем переменные окружения или значения по умолчанию
    host := getEnv("POSTGRES_HOST", "localhost")
    port := getEnv("POSTGRES_PORT", "5432")
    user := getEnv("POSTGRES_USER", "validator")
    password := getEnv("POSTGRES_PASSWORD", "val1dat0r")
    dbname := getEnv("POSTGRES_DB", "project-sem-1")
    
    connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
        user, password, dbname, host, port)
    
    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    defer db.Close()

    // Ждем подключения
    for i := 0; i < 10; i++ {
        if err := db.Ping(); err == nil {
            break
        }
        time.Sleep(1 * time.Second)
    }
    
    if err := db.Ping(); err != nil {
        log.Fatal("Database connection failed:", err)
    }

    createTable()

    r := mux.NewRouter()
    r.HandleFunc("/api/v0/prices", handlePrices).Methods("POST", "GET")

    serverPort := getEnv("PORT", "8080")
    log.Printf("Server starting on :%s", serverPort)
    log.Fatal(http.ListenAndServe(":"+serverPort, r))
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func createTable() {
    // структура таблицы
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
        log.Printf("Warning creating table: %v", err)
    }
}

func handlePrices(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        handlePost(w, r)
    } else if r.Method == http.MethodGet {
        handleGet(w, r)
    } else {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func handlePost(w http.ResponseWriter, r *http.Request) {
    var reader io.Reader
    var err error
    
    // Поддержка multipart/form-data
    if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
        err = r.ParseMultipartForm(10 << 20)
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
        reader = file
    } else {
        // Поддержка raw body
        reader = r.Body
        defer r.Body.Close()
    }
    
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

    // ищем csv
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

    // столбцы для вставки
    stmt, err := tx.Prepare(`
        INSERT INTO prices (name, category, price, create_date) 
        VALUES ($1, $2, $3, $4)
    `)
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer stmt.Close()

    // Пропускаем заголовок, если есть
    start := 0
    if len(records) > 0 && strings.ToLower(records[0][0]) == "id" {
        start = 1
    }

    inserted := 0
    for i := start; i < len(records); i++ {
        record := records[i]
        if len(record) < 5 {
            continue
        }

        // порядок столбцов id, name, category, price, create_date
        productID, _ := strconv.Atoi(strings.TrimSpace(record[0]))
        name := strings.TrimSpace(record[1])
        category := strings.TrimSpace(record[2])
        priceStr := strings.TrimSpace(record[3])
        dateStr := strings.TrimSpace(record[4])

        price, err := strconv.ParseFloat(priceStr, 64)
        if err != nil {
            continue // пропускаем некорректные цены
        }

        // парсим дату
        createDate, err := time.Parse("2006-01-02", dateStr)
        if err != nil {
            continue // пропускаем некорректные даты
        }

        _, err = stmt.Exec(name, category, price, createDate)
        if err != nil {
            continue
        }

        inserted++
        stats.TotalItems++
        categorySet[category] = true
        stats.TotalPrice += price
    }

    if inserted == 0 {
        http.Error(w, "No valid data to insert", http.StatusBadRequest)
        return
    }

    if err := tx.Commit(); err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    stats.TotalCategories = len(categorySet)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(stats)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
    // столбцы для выборки
    rows, err := db.Query(`
        SELECT id, name, category, price, create_date 
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
        http.Error(w, "Failed to create CSV", http.StatusInternalServerError)
        return
    }
    defer os.Remove(csvFile.Name())
    defer csvFile.Close()

    csvWriter := csv.NewWriter(csvFile)
    // заголовки столбцов в csv
    csvWriter.Write([]string{"id", "name", "category", "price", "create_date"})
    
    for rows.Next() {
        var id int
        var name, category string
        var price float64
        var createDate time.Time
        
        if err := rows.Scan(&id, &name, &category, &price, &createDate); err != nil {
            continue
        }
        
        // формат даты
        record := []string{
            strconv.Itoa(id),
            name,
            category,
            strconv.FormatFloat(price, 'f', 2, 64),
            createDate.Format("2006-01-02"),
        }
        
        csvWriter.Write(record)
    }
    
    if err := rows.Err(); err != nil {
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
        http.Error(w, "Failed to create ZIP", http.StatusInternalServerError)
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
}
