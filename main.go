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
            time.Sleep(1 * time.Second)
        } else {
            break
        }
    }
    
    if err := db.Ping(); err != nil {
        log.Fatal("Database ping failed after retries:", err)
    }
    log.Println("Connected to PostgreSQL database")

    createTable()

    http.HandleFunc("/api/v0/prices", handlePrices)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    log.Println("Server starting on :" + port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
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
        err := r.ParseMultipartForm(10 << 20) // 10 MB
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
    
    // Если не multipart, читаем напрямую
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
    
    // Ищем CSV файл в архиве
    var csvFile *zip.File
    for _, f := range zipReader.File {
        baseName := filepath.Base(f.Name)
        if baseName == "data.csv" || baseName == "test_data.csv" {
            csvFile = f
            break
        }
    }
    if csvFile == nil {
        http.Error(w, "CSV file not found in archive", http.StatusBadRequest)
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
    
    // Определяем есть ли заголовок
    start := 0
    if len(records) > 0 {
        // Проверяем, первая ли строка - заголовок
        firstRecord := strings.Join(records[0], ",")
        firstRecordLower := strings.ToLower(firstRecord)
        if strings.Contains(firstRecordLower, "id") && 
           (strings.Contains(firstRecordLower, "name") || strings.Contains(firstRecordLower, "date")) {
            start = 1
        }
    }
    
    // попробуем оба варианта парсинга
    for i := start; i < len(records); i++ {
        record := records[i]
        if len(record) < 5 {
            continue
        }

        var name, category, dateStr, priceStr string
        
        // Пробуем определить формат по первому полю (id всегда число)
        if _, err := strconv.Atoi(strings.TrimSpace(record[0])); err == nil {
            // Первое поле - число, значит это id
            // Пробуем определить формат по остальным полям
            
            // Если второе поле похоже на дату (содержит "-")
            if strings.Contains(record[1], "-") && len(strings.Split(record[1], "-")) == 3 {
                // Формат: id, create_date, name, category, price
                dateStr = strings.TrimSpace(record[1])
                name = strings.TrimSpace(record[2])
                category = strings.TrimSpace(record[3])
                priceStr = strings.TrimSpace(record[4])
            } else {
                // Формат: id, name, category, price, create_date
                name = strings.TrimSpace(record[1])
                category = strings.TrimSpace(record[2])
                priceStr = strings.TrimSpace(record[3])
                dateStr = strings.TrimSpace(record[4])
            }
        } else {
            // Неопределенный формат, пробуем по умолчанию
            name = strings.TrimSpace(record[0])
            category = strings.TrimSpace(record[1])
            priceStr = strings.TrimSpace(record[2])
            dateStr = strings.TrimSpace(record[3])
        }

        // Пропускаем пустые строки
        if name == "" || category == "" || priceStr == "" || dateStr == "" {
            continue
        }

        price, err := strconv.ParseFloat(priceStr, 64)
        if err != nil {
            continue
        }

        // Парсим дату
        var createDate time.Time
        createDate, err = time.Parse("2006-01-02", dateStr)
        if err != nil {
            // Пробуем другие форматы даты
            createDate, err = time.Parse("2006/01/02", dateStr)
            if err != nil {
                continue
            }
        }

        _, err = stmt.Exec(name, category, price, createDate)
        if err != nil {
            log.Printf("Insert error: %v", err)
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
    if err := json.NewEncoder(w).Encode(stats); err != nil {
        log.Printf("Failed to encode response: %v", err)
    }
}

func handleGet(w http.ResponseWriter, r *http.Request) {
    log.Println("GET /api/v0/prices request received")

    // Выбираем все записи
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

    // Создаем временный CSV файл
    csvFile, err := os.CreateTemp("", "data-*.csv")
    if err != nil {
        http.Error(w, "Failed to create CSV file", http.StatusInternalServerError)
        return
    }
    defer os.Remove(csvFile.Name())
    defer csvFile.Close()

    csvWriter := csv.NewWriter(csvFile)
    
    // Записываем заголовок
    if err := csvWriter.Write([]string{"id", "name", "category", "price", "create_date"}); err != nil {
        http.Error(w, "Failed to write CSV header", http.StatusInternalServerError)
        return
    }
    
    // Читаем данные из БД
    for rows.Next() {
        var id int
        var name, category string
        var price float64
        var createDate time.Time
        
        if err := rows.Scan(&id, &name, &category, &price, &createDate); err != nil {
            log.Printf("Scan error: %v", err)
            continue
        }
        
        // Формируем запись
        record := []string{
            strconv.Itoa(id),
            name,
            category,
            strconv.FormatFloat(price, 'f', 2, 64),
            createDate.Format("2006-01-02"),
        }
        
        if err := csvWriter.Write(record); err != nil {
            http.Error(w, "Failed to write CSV record", http.StatusInternalServerError)
            return
        }
    }
    
    // Проверяем ошибки после цикла
    if err := rows.Err(); err != nil {
        http.Error(w, "Database cursor error", http.StatusInternalServerError)
        return
    }
        
    csvWriter.Flush()
    if err := csvWriter.Error(); err != nil {
        http.Error(w, "Failed to flush CSV writer", http.StatusInternalServerError)
        return
    }

    // Перемещаем указатель в начало файла
    csvFile.Seek(0, 0)

    // Создаем временный ZIP файл
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

    // Копируем CSV в ZIP
    if _, err := io.Copy(fileInZip, csvFile); err != nil {
        http.Error(w, "Failed to copy CSV to ZIP", http.StatusInternalServerError)
        return
    }
    
    // Закрываем ZIP writer
    if err := zipWriter.Close(); err != nil {
        http.Error(w, "Failed to close ZIP writer", http.StatusInternalServerError)
        return
    }

    // Перемещаем указатель в начало ZIP файла
    zipFile.Seek(0, 0)

    // Отправляем ответ
    w.Header().Set("Content-Type", "application/zip")
    w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
    
    if _, err := io.Copy(w, zipFile); err != nil {
        log.Printf("Failed to send ZIP file: %v", err)
    }
    
    log.Println("ZIP archive sent successfully")
}
