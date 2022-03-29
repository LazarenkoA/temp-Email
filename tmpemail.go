package tmpemail

import (
	"context"
	"fmt"
	"golang.org/x/net/proxy"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

type Result struct {
	Email string
	Error error
}
type Proxy struct {
	address string
	port    string
}

type TmpEmailConf struct {
	// канал для результата
	Result chan *Result
	// Таймаут в течение которого будет ожидаться письмо с подтверждением
	Timeout time.Duration
	// функция для обработки входящих сообщений
	Activation func(from, body string) bool
	// Прокси
	Proxy *Proxy
}

type ITmpEmail interface {
	NewRegistration() error
}

func (t *TmpEmailConf) getResponse(url string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := t.httpClient(time.Second * 30)

	if resp, err := client.Do(req); err != nil {
		return []byte{}, fmt.Errorf("Регистрация нового email. Произошла ошибка при выполнении запроса:\n%q \n", err.Error())
	} else if resp.StatusCode-(resp.StatusCode%100) != 200 {
		return []byte{}, fmt.Errorf("Код ответа: %d \n", resp.StatusCode)
	} else {
		body, _ := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		return body, nil
	}
}

func (t *TmpEmailConf) httpClient(timeout time.Duration) *http.Client {
	httpTransport := &http.Transport{}
	if t.Proxy != nil {
		httpTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			select {
			case <-ctx.Done():
				return nil, nil
			default:
			}

			dialer, err := proxy.SOCKS5("tcp", t.Proxy.address+":"+t.Proxy.port, nil, proxy.Direct)
			if err != nil {
				return nil, err
			}

			return dialer.Dial(network, addr)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: httpTransport,
	}
}
