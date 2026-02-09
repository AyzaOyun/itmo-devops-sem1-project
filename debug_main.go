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
    "strconv"
    "strings"
    "time"
    _ "github.com/lib/pq"
)

var db *sql.DB

func main() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    
    // Подключение
    connStr := "user=validator password=val1dat0r dbname=project-sem-1 host=localhost port=5432 sslmode=disable"
    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("DB open error:", err)
    }
    defer db.Close()
    
    // Ждем БД
    for i := 0; i < 5; i++ {
        if err = db.Ping(); err != nil {
            log.Printf("Ping attempt %d: %v", i+1, err)
            time.Sleep(2 * time.Second)
        } else {
            break
        }
    }
    
    if err = db.Ping(); err != nil {
        log.Fatal("DB ping failed:", err)
    }
    
    // Создаем таблицу
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS prices (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            category TEXT NOT NULL,
            price DECIMAL(10,2) NOT NULL,
            create_date TIMESTAMP NOT NULL
        )
    `)
    if err != nil {
        log.Printf("Table creation error: %v", err)
    }
    
    http.HandleFunc("/api/v0/prices", handler)
    log.Println("Starting server on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
    log.Printf("%s %s", r.Method, r.URL.Path)
    
    if r.Method == "POST" {
        handlePost(w, r)
    } else if r.Method == "GET" {
        handleGet(w, r)
    } else {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func handlePost(w http.ResponseWriter, r *http.Request) {
    log.Println("=== POST REQUEST ===")
    log.Printf("Content-Type: %s", r.Header.Get("Content-Type"))
    log.Printf("Content-Length: %s", r.Header.Get("Content-Length"))
    
    // Парсим multipart
    err := r.ParseMultipartForm(10 << 20) // 10MB
    if err != nil {
        log.Printf("ParseMultipartForm error: %v", err)
        http.Error(w, "Bad request", http.StatusBadRequest)
        return
    }
    
    file, header, err := r.FormFile("file")
    if err != nil {
        log.Printf("FormFile error: %v", err)
        http.Error(w, "No file", http.StatusBadRequest)
        return
    }
    defer file.Close()
    
    log.Printf("File header: %+v", header)
    
    // Читаем весь файл
    data, err := io.ReadAll(file)
    if err != nil {
        log.Printf("ReadAll error: %v", err)
        http.Error(w, "Read error", http.StatusInternalServerError)
        return
    }
    
    log.Printf("File size: %d bytes", len(data))
    
    // Открываем ZIP
    zipReader, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
    if err != nil {
        log.Printf("ZIP error: %v", err)
        http.Error(w, "Invalid ZIP", http.StatusBadRequest)
        return
    }
    
    log.Printf("Files in ZIP: %d", len(zipReader.File))
    for i, f := range zipReader.File {
        log.Printf("  [%d] %s (compressed: %d, uncompressed: %d)", 
            i, f.Name, f.CompressedSize64, f.UncompressedSize64)
    }
    
    // Ищем CSV
    var csvFile *zip.File
    for _, f := range zipReader.File {
        if strings.HasSuffix(f.Name, ".csv") {
            csvFile = f
            break
        }
    }
    
    if csvFile == nil {
        log.Println("No CSV file found")
        http.Error(w, "No CSV in ZIP", http.StatusBadRequest)
        return
    }
    
    log.Printf("Using CSV file: %s", csvFile.Name)
    
    // Открываем CSV
    rc, err := csvFile.Open()
    if err != nil {
        log.Printf("CSV open error: %v", err)
        http.Error(w, "CSV error", http.StatusInternalServerError)
        return
    }
    defer rc.Close()
    
    // Читаем CSV
    reader := csv.NewReader(rc)
    records, err := reader.ReadAll()
    if err != nil {
        log.Printf("CSV read error: %v", err)
        http.Error(w, "CSV parse error", http.StatusBadRequest)
        return
    }
    
    log.Printf("CSV records: %d", len(records))
    for i, record := range records {
        log.Printf("Record[%d]: %v", i, record)
    }
    
    // Обработка данных
    if len(records) == 0 {
        log.Println("Empty CSV")
        http.Error(w, "Empty CSV", http.StatusBadRequest)
        return
    }
    
    // Определяем есть ли заголовок
    start := 0
    if len(records) > 0 {
        firstRecord := strings.ToLower(strings.Join(records[0], " "))
        if strings.Contains(firstRecord, "id") && strings.Contains(firstRecord, "name") {
            log.Println("CSV has header")
            start = 1
        }
    }
    
    // Вставляем данные
    count := 0
    categories := make(map[string]bool)
    totalPrice := 0.0
    
    for i := start; i < len(records); i++ {
        record := records[i]
        if len(record) < 5 {
            log.Printf("Record %d too short: %v", i, record)
            continue
        }
        
        // ПОРЯДОК ВОПРОС: Посмотрим что в данных
        log.Printf("Processing record %d: %v", i, record)
        
        // В тестах: id,name,category,price,create_date
        name := strings.TrimSpace(record[1])
        category := strings.TrimSpace(record[2])
        priceStr := strings.TrimSpace(record[3])
        dateStr := strings.TrimSpace(record[4])
        
        log.Printf("  Parsed: name='%s', category='%s', price='%s', date='%s'", 
            name, category, priceStr, dateStr)
        
        price, err := strconv.ParseFloat(priceStr, 64)
        if err != nil {
            log.Printf("  Price parse error: %v", err)
            continue
        }
        
        date, err := time.Parse("2006-01-02", dateStr)
        if err != nil {
            log.Printf("  Date parse error: %v", err)
            continue
        }
        
        // Вставка
        _, err = db.Exec(
            "INSERT INTO prices (name, category, price, create_date) VALUES ($1, $2, $3, $4)",
            name, category, price, date,
        )
        
        if err != nil {
            log.Printf("  DB insert error: %v", err)
        } else {
            count++
            categories[category] = true
            totalPrice += price
            log.Printf("  Inserted: %s, %s, %.2f, %s", name, category, price, date.Format("2006-01-02"))
        }
    }
    
    // Формируем ответ
    response := map[string]interface{}{
        "total_items":      count,
        "total_categories": len(categories),
        "total_price":      totalPrice,
    }
    
    log.Printf("Response: %v", response)
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
    log.Println("=== POST RESPONSE SENT ===")
}

func handleGet(w http.ResponseWriter, r *http.Request) {
    log.Println("=== GET REQUEST ===")
    // Пока просто ответ для теста
    w.Write([]byte(`{"message": "GET works"}`))
}
