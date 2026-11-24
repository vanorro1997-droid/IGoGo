package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider/cloudflare"
)

// Config — параметры клиента. Если какое-то поле пустое,
// используются значения по умолчанию (см. New).
type Config struct {
	APIKey       string
	AccountID    string
	Model        string        // "" → @cf/google/gemma-4-26b-a4b-it
	Proxy        string        // "" → прямое подключение
	SystemPrompt string        // "" → без системного сообщения
	Timeout      time.Duration // 0 → 60s
}

// Result — результат одного запроса.
type Result struct {
	Text string
	Err  error
}

// Client — клиент Workers AI.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewFromEnv читает .env (если есть) и переменные окружения,
// создаёт клиент. Это основной способ инициализации.
func NewFromEnv() (*Client, error) {
	_ = godotenv.Load() // отсутствие .env — не ошибка

	proxy := os.Getenv("CLOUDFLARE_PROXY")
	if proxy == "" {
		proxy = os.Getenv("HTTPS_PROXY")
	}

	return New(Config{
		APIKey:       os.Getenv("CLOUDFLARE_API_TOKEN"),
		AccountID:    os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		Model:        os.Getenv("CLOUDFLARE_MODEL"),
		Proxy:        proxy,
		SystemPrompt: os.Getenv("CLOUDFLARE_SYSTEM_PROMPT"),
	})
}

// New создаёт клиент с явной конфигурацией.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm: не задан APIKey (CLOUDFLARE_API_TOKEN)")
	}
	if cfg.AccountID == "" {
		return nil, fmt.Errorf("llm: не задан AccountID (CLOUDFLARE_ACCOUNT_ID)")
	}
	if cfg.Model == "" {
		cfg.Model = "@cf/google/gemma-4-26b-a4b-it"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("llm: Timeout должен быть положительным")
	}

	// Транспорт: прокси + увеличенный TLS-таймаут (помогает из RU).
	transport := &http.Transport{
		TLSHandshakeTimeout: 30 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil || proxyURL.Host == "" {
			return nil, fmt.Errorf("llm: некорректный адрес прокси")
		}
		switch proxyURL.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("llm: неподдерживаемый протокол прокси")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Transport: transport, Timeout: cfg.Timeout},
	}, nil
}

// SystemPrompt возвращает текущий системный промпт.
func (c *Client) SystemPrompt() string { return c.cfg.SystemPrompt }

// Model возвращает используемую модель.
func (c *Client) Model() string { return c.cfg.Model }

// Ask синхронно отправляет запрос и возвращает текст ответа.
func (c *Client) Ask(prompt string) (string, error) {
	return c.AskCtx(context.Background(), prompt)
}

// AskCtx — синхронный вызов с возможностью отмены через ctx.
func (c *Client) AskCtx(parent context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, c.cfg.Timeout)
	defer cancel()

	model := cloudflare.Chat(c.cfg.Model,
		cloudflare.WithAPIKey(c.cfg.APIKey),
		cloudflare.WithAccountID(c.cfg.AccountID),
		cloudflare.WithHTTPClient(c.httpClient),
	)

	opts := make([]goai.Option, 0, 2)
	if c.cfg.SystemPrompt != "" {
		opts = append(opts, goai.WithSystem(c.cfg.SystemPrompt))
	}
	opts = append(opts, goai.WithPrompt(prompt))

	res, err := goai.GenerateText(ctx, model, opts...)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// AskAsync запускает запрос в горутине и возвращает канал с результатом.
// Канал буферизован на 1 и закрывается после отправки значения.
func (c *Client) AskAsync(prompt string) <-chan Result {
	return c.AskAsyncCtx(context.Background(), prompt)
}

// AskAsyncCtx — асинхронный вызов с возможностью отмены через ctx.
func (c *Client) AskAsyncCtx(parent context.Context, prompt string) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		defer close(ch)
		text, err := c.AskCtx(parent, prompt)
		ch <- Result{Text: text, Err: err}
	}()
	return ch
}

/*
Используется бесплатный API CLOUDFLARE
https://dash.cloudflare.com/04ac02c5282e3ea989b182da3b025de0/ai/workers-ai/api-quick-start
регистрация бесплатная
но для использования API нужен прокси клиент типа v2reyN / Happ
(из-за ограниичениия трафика к CLOUDFLARE коробочками у провайдеров)
без прокси падает ошибка timeout


├── main.go
├── .env
└── internal/providers/llm/
    └── llm.go       ← пакет




.env
CLOUDFLARE_API_TOKEN=ваш_api_token
CLOUDFLARE_ACCOUNT_ID=ваш_account_id
CLOUDFLARE_MODEL=@cf/google/gemma-4-26b-a4b-it
CLOUDFLARE_PROXY=http://127.0.0.1:10808
CLOUDFLARE_SYSTEM_PROMPT=Ты — лаконичный технический ассистент. Отвечай по-русски, кратко и по делу.




пример использования (import "github.com/vanorro1997-droid/IGoGo/internal/providers/llm")

func main() {
	client, err := llm.NewFromEnv()
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	fmt.Printf("model: %s\n", client.Model())
	if sp := client.SystemPrompt(); sp != "" {
		fmt.Printf("system: %s\n", sp)
	}
	fmt.Println("Введите запрос (/exit — выход):")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}
		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" {
			continue
		}
		if prompt == "/exit" || prompt == "/quit" {
			return
		}

		res := <-client.AskAsync(prompt)
		if res.Err != nil {
			fmt.Println("ошибка:", res.Err)
		} else {
			fmt.Println(res.Text)
		}
	}
}

*/
