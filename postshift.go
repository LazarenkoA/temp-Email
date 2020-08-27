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
	Error error
}

type TmpEmailConf struct {
	Result     chan *Result
	Timeout    time.Duration
	Activation func(from, body string) bool
	Proxy *struct{
		address string
		port string
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
			close(t.conf.Result)
			t.clearEmail()
		}
	}

	return nil
}

func (t *TmpEmail) watcherMail() {
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(t.conf.Timeout))
	tick := time.NewTicker(time.Second * 2)
	defer tick.Stop()

	checked := map[int]bool{}

FOR:
	for {
		t.readInBox(checked)

		select {
		case <-ctx.Done():
			t.conf.Result <-  &Result{
				Error: errors.New("Прервано по таймауту"),
			}
			close(t.conf.Result)
			t.clearEmail()
			break FOR
		case <-tick.C:
		default:

		}
	}
}

func (t *TmpEmail) readInBox(checked map[int]bool) (result string) {
	// EAFP
	defer func() {
		if err := recover(); err != nil {
			result = ""
		}
	}()

	if body, err := t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=getlist&key=%v&type=json", t.key)); err == nil {
		tmp := []map[string]interface{}{}
		if err := json.Unmarshal(body, &tmp); err != nil {
			//log.Printf("Получение списка писем. Ошибка сериализации json: %q \n", err.Error())
			return ""
		}

		for _, body := range tmp {
			if from, ok := body["from"]; ok {
				id := int(body["id"].(float64))
				if !checked[id] {
					checked[id] = true
					t.readEmail(from.(string), id)
				}
			}
		}
	}
	return ""
}

func (t *TmpEmail) httpClient(timeout time.Duration) *http.Client {
	httpTransport := &http.Transport{}
	if t.conf.Proxy != nil {
		//logrus.Debug("Используем прокси " + net_.PROXY_ADDR)
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
		Timeout: timeout,
		Transport: httpTransport,
	}
}

func (t *TmpEmail) getResponse(url string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	client := t.httpClient(time.Minute * 2)

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

func (t *TmpEmail) readEmail(from string, id int) {
	if body, err := t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=getmail&key=%v&id=%d", t.key, id)); err == nil {
		if t.conf.Activation(from, string(body)) {
			t.conf.Result <- &Result{
				Email:   t.email,
				Confirm: true,
			}
			close(t.conf.Result)
			t.clearEmail()
		}
	}
}

func (t *TmpEmail) clearEmail() {
	t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=clear&key=%v", t.key))
}
