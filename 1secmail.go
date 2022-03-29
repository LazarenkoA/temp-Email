package tmpemail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OneSecmail struct {
	email         string
	login, domain string

	conf *TmpEmailConf
	ctx  context.Context
}

func (t *OneSecmail) Create(conf *TmpEmailConf) ITmpEmail {
	t.conf = conf
	return t
}

func (t *OneSecmail) NewRegistration() error {
	if body, err := t.conf.getResponse("https://www.1secmail.com/api/v1/?action=genRandomMailbox&count=1"); err != nil {
		log.Printf("Регистрация нового email. Ошибка:\n %q \n", err.Error())
		return err
	} else {
		tmp := []string{}
		if err := json.Unmarshal(body, &tmp); err != nil {
			return fmt.Errorf("Регистрация нового email. Ошибка сериализации json: %q \n", err.Error())
		}

		if len(tmp) == 0 {
			return errors.New("не удалось зарегистрировать временную почту")
		}
		t.email = tmp[0]
		parts := strings.Split(t.email, "@")
		if len(parts) != 2 {
			return errors.New("email не корректного формата")
		}
		t.login = parts[0]
		t.domain = parts[1]

		t.conf.Result <- &Result{
			Email: t.email,
		}

		if t.conf.Activation != nil {
			t.ctx, _ = context.WithTimeout(context.Background(), t.conf.Timeout)
			go t.watcherMail() // запускаем горутину что б она проверяла входящие письма
		} else {
			t.deleteEmail()
			close(t.conf.Result)
		}

	}

	return nil
}

func (t *OneSecmail) watcherMail() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	//FOR:
	for range tick.C {
		if t.readInBox() {
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

func (t *OneSecmail) readInBox() (result bool) {
	if body, err := t.conf.getResponse(fmt.Sprintf("https://www.1secmail.com/api/v1/?action=getMessages&login=%s&domain=%s", t.login, t.domain)); err == nil {
		emails := []map[string]interface{}{}
		if err := json.Unmarshal(body, &emails); err != nil {
			return false
		}
		for _, em := range emails {
			if b := t.getBody(em["id"].(float64)); b != "" {
				return t.conf.Activation(em["from"].(string), b)
			}
		}
	}

	return false
}

func (t *OneSecmail) deleteEmail() {
	delUrl := "https://www.1secmail.com/mailbox"
	data := url.Values{}
	data.Set("action", "deleteMailbox")

	data.Set("login", t.login)
	data.Set("domain", t.domain)
	http.PostForm(delUrl, data)
}

func (t *OneSecmail) getBody(id float64) string {
	if body, err := t.conf.getResponse(fmt.Sprintf("https://www.1secmail.com/api/v1/?action=readMessage&login=%s&domain=%s&id=%f", t.login, t.domain, id)); err == nil {
		email_body := map[string]interface{}{}
		if err := json.Unmarshal(body, &email_body); err != nil {
			return ""
		}

		return email_body["body"].(string)
	}
	return ""
}
