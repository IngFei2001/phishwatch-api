package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

var db *sql.DB

type GreyURL struct {
	ID         int    `json:"id"`
	URLPattern string `json:"url_pattern"`
	RiskScore  int    `json:"risk_score"`
}

func main() {
	// 连接数据库
	var err error
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://neondb_owner:npg_MTkL9RNh1Zzl@ep-falling-haze-a1citxdd-pooler.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	}

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}
	defer db.Close()

	// 测试连接
	if err = db.Ping(); err != nil {
		log.Fatal("数据库 Ping 失败:", err)
	}
	log.Println("数据库连接成功!")

	// 设置路由
	r := gin.Default()

	// API 路由
	r.GET("/api/urls", getAllURLs)
	r.GET("/api/check", checkURL)
	r.POST("/api/urls", addURL)
	r.DELETE("/api/urls/:id", deleteURL)

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("服务器启动在 http://localhost:%s", port)
	r.Run(":" + port)
}

// GET /api/urls - 获取所有 URL
func getAllURLs(c *gin.Context) {
	rows, err := db.Query("SELECT id, url_pattern, risk_score FROM grey_urls ORDER BY id")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var urls []GreyURL
	for rows.Next() {
		var u GreyURL
		if err := rows.Scan(&u.ID, &u.URLPattern, &u.RiskScore); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		urls = append(urls, u)
	}

	c.JSON(200, urls)
}

// GET /api/check?url=xxx - 检查 URL 是否在灰名单
func checkURL(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(400, gin.H{"error": "缺少 url 参数"})
		return
	}

	var u GreyURL
	err := db.QueryRow("SELECT id, url_pattern, risk_score FROM grey_urls WHERE url_pattern = $1", url).Scan(&u.ID, &u.URLPattern, &u.RiskScore)

	if err == sql.ErrNoRows {
		c.JSON(200, gin.H{"found": false, "url": url, "message": "URL 不在灰名单中"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"found": true, "url": u.URLPattern, "risk_score": u.RiskScore})
}

// POST /api/urls - 添加新 URL
func addURL(c *gin.Context) {
	var input struct {
		URLPattern string `json:"url_pattern"`
		RiskScore  int    `json:"risk_score"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "无效的 JSON 数据"})
		return
	}

	// 保护：只接受 8-10 分
	if input.RiskScore < 8 || input.RiskScore > 10 {
		c.JSON(400, gin.H{"error": "分数必须 8-10 才能存入"})
		return
	}

	// 检查 URL 是否已存在
	var existingID int
	var existingScore int
	err := db.QueryRow("SELECT id, risk_score FROM grey_urls WHERE url_pattern = $1", input.URLPattern).Scan(&existingID, &existingScore)

	if err == sql.ErrNoRows {
		// URL 不存在，插入新记录
		var id int
		err := db.QueryRow("INSERT INTO grey_urls (url_pattern, risk_score) VALUES ($1, $2) RETURNING id", input.URLPattern, input.RiskScore).Scan(&id)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, gin.H{"message": "URL 已添加", "id": id, "risk_score": input.RiskScore})
		return
	}

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// URL 已存在，检查分数是否更高
	if input.RiskScore > existingScore {
		// 新分数更高，更新
		_, err := db.Exec("UPDATE grey_urls SET risk_score = $1 WHERE id = $2", input.RiskScore, existingID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "分数已更新", "id": existingID, "old_score": existingScore, "new_score": input.RiskScore})
		return
	}

	// 新分数不够高，不更新
	c.JSON(200, gin.H{"message": "分数未更新（新分数不高于现有分数）", "id": existingID, "current_score": existingScore, "submitted_score": input.RiskScore})
}

// DELETE /api/urls/:id - 删除 URL
func deleteURL(c *gin.Context) {
	id := c.Param("id")

	result, err := db.Exec("DELETE FROM grey_urls WHERE id = $1", id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(404, gin.H{"error": "URL 不存在"})
		return
	}

	c.JSON(200, gin.H{"message": "URL 已删除"})
}