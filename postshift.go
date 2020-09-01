package postshift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

type Result struct {
	Email   string
	Confirm bool
	Error   error
}

type TmpEmailConf struct {
	// канал для результата
	Result chan *Result
	// Таймаут в течении которого будет ожидаться письмо с подтверждением
	Timeout time.Duration
	// функция для обработки входящих сообщений
	Activation func(from, body string) bool
	// Прокси
	Proxy *struct {
		address string
		port    string
	}
}

type TmpEmail struct {
	email string
	key   string
	conf  *TmpEmailConf
}

func (t *TmpEmail) Create(conf *TmpEmailConf) *TmpEmail {
	t.conf = conf
	return t
}

// Регистрация новой почты
// если параметр confirm = true должна быть задана функция Activation
func (t *TmpEmail) NewRegistration(confirm bool) error {
	if body, err := t.getResponse("https://post-shift.ru/api.php?action=new&type=json"); err != nil {
		log.Printf("Регистрация нового email. Ошибка:\n %q \n", err.Error())
		return err
	} else {
		tmp := map[string]interface{}{}
		if err := json.Unmarshal(body, &tmp); err != nil {
			return fmt.Errorf("Регистрация нового email. Ошибка сериализации json: %q \n", err.Error())
		}

		if e, ok := tmp["error"]; ok {
			return errors.New(e.(string))
		}

		t.email = tmp["email"].(string)
		t.key = tmp["key"].(string)

		t.conf.Result <- &Result{
			Email:   t.email,
			Confirm: false,
		}

		// запускаем горутину что б она проверяла входящие письма
		if confirm {
			if t.conf.Activation == nil {
				return errors.New("Должна быть задана функция активации")
			}

			go t.watcherMail()
		} else {
			t.deleteEmail()
			close(t.conf.Result)
		}
	}

	return nil
}

func (t *TmpEmail) watcherMail() {
	tick := time.NewTicker(time.Second * 2)
	defer tick.Stop()

	checked := map[int]bool{}

FOR:
	for range tick.C {
		if t.readInBox(checked) {
			t.deleteEmail()
			t.conf.Result <- &Result{
				Email:   t.email,
				Confirm: true,
			}
			close(t.conf.Result)
			break
		}

		select {
		case <-time.After(t.conf.Timeout):
			t.deleteEmail()

			t.conf.Result <- &Result{
				Error: errors.New("Прервано по таймауту"),
			}

			close(t.conf.Result)
			break FOR
		case <-tick.C:
		}
	}
}

func (t *TmpEmail) readInBox(checked map[int]bool) (result bool) {
	// EAFP
	defer func() {
		if err := recover(); err != nil {
			result = false
		}
	}()

	if body, err := t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=getlist&key=%v&type=json", t.key)); err == nil {
		tmp := []map[string]interface{}{}
		if err := json.Unmarshal(body, &tmp); err != nil {
			return false
		}

		for _, body := range tmp {
			if from, ok := body["from"]; ok {
				id := int(body["id"].(float64))
				if !checked[id] {
					checked[id] = true
					return t.readEmail(from.(string), id)
				}
			}
		}
	}
	return false
}

func (t *TmpEmail) httpClient(timeout time.Duration) *http.Client {
	httpTransport := &http.Transport{}
	if t.conf.Proxy != nil {
		httpTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			select {
			case <-ctx.Done():
				return nil, nil
			default:
			}

			dialer, err := proxy.SOCKS5("tcp", t.conf.Proxy.address+":"+t.conf.Proxy.port, nil, proxy.Direct)
			if err != nil {
				//logrus.WithField("Прокси", net_.PROXY_ADDR).Errorf("Ошибка соединения с прокси: %q", err)
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

func (t *TmpEmail) getResponse(url string) ([]byte, error) {
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

func (t *TmpEmail) readEmail(from string, id int) bool {
	if body, err := t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=getmail&key=%v&id=%d", t.key, id)); err == nil {
		return t.conf.Activation(from, string(body))
	}
	return false
}

func (t *TmpEmail) deleteEmail() {
	t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=delete&key=%v", t.key))
}

func (t *TmpEmail) DeleteAllEmail() {
	t.getResponse("https://post-shift.ru/api.php?action=deleteall")
}
