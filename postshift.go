package tmpemail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"log"
	"net"
	"net/http"
	"time"
)

type PostShift struct {
	TmpEmailConf

	email string
	key   string
	conf  *TmpEmailConf
	ctx   context.Context
}

func (t *PostShift) Create(conf *TmpEmailConf) ITmpEmail {
	t.conf = conf
	return t
}

func (t *PostShift) NewRegistration() error {
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
			Email: t.email,
		}

		// запускаем горутину что б она проверяла входящие письма
		//if confirm {
		//	if t.conf.Activation == nil {
		//		return errors.New("Должна быть задана функция активации")
		//	}
		//	t.ctx, _ = context.WithTimeout(context.Background(), t.conf.Timeout)
		//
		//	go t.watcherMail()
		//} else {
		//	t.deleteEmail()
		//	close(t.conf.Result)
		//}
	}

	return nil
}

func (t *PostShift) watcherMail() {
	tick := time.NewTicker(time.Second * 2)
	defer tick.Stop()

	checked := map[int]bool{}

	//FOR:
	for range tick.C {
		if t.readInBox(checked) {
			t.deleteEmail()
			t.conf.Result <- &Result{
				Email: t.email,
			}
			close(t.conf.Result)
			break
		}

		if errors.Is(t.ctx.Err(), context.DeadlineExceeded) {
			t.deleteEmail()

			t.conf.Result <- &Result{
				Error: errors.New("Прервано по таймауту"),
			}

			close(t.conf.Result)
			break
		}

		//select {
		//case <-t.ctx.Done():
		//	break FOR
		//case <-tick.C:
		//}
	}
}

func (t *PostShift) readInBox(checked map[int]bool) (result bool) {
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

func (t *PostShift) httpClient(timeout time.Duration) *http.Client {
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

func (t *PostShift) readEmail(from string, id int) bool {
	if body, err := t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=getmail&key=%v&id=%d", t.key, id)); err == nil {
		return t.conf.Activation(from, string(body))
	}
	return false
}

func (t *PostShift) deleteEmail() {
	t.getResponse(fmt.Sprintf("https://post-shift.ru/api.php?action=delete&key=%v", t.key))
}

func (t *PostShift) DeleteAllEmail() {
	t.getResponse("https://post-shift.ru/api.php?action=deleteall")
}
