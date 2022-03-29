package main

import (
	"fmt"
	temail "github.com/LazarenkoA/temp-Email"
	"regexp"
	"strings"
	"time"
)

func main() {
	reg := regexp.MustCompile(`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&\/=]*)`)
	factivation := func(from, body string) bool {
		m := reg.FindStringSubmatch(body) // берем url из письма и переходим по нему
		if len(m) > 0 {
			// getRequest(m[0]) // GET запрос
		}

		// Если функция возвращает true это значит что почта подтверждена и нам она больше не нужна.
		// После подтверждения или по таймауту (задается в настройках) временная почта удаляется
		return strings.Contains(from, "admin@mail.ru") && len(m) > 0
	}

	cResult := make(chan *temail.Result, 1) // размер канала обязательно должен быть 1 или больше
	newEmail := new(temail.OneSecmail).Create(&temail.TmpEmailConf{
		Result:     cResult,         // канал для результата
		Timeout:    time.Minute * 2, // Таймаут в течение которого будет ожидаться письмо с подтверждением
		Activation: factivation,     // функция для обработки входящих сообщений, если обработка не нужна, этот параметр не указывается
	})

	if err := newEmail.NewRegistration(); err != nil {
		fmt.Println("Произошла ошибка при регистрации почты ", err.Error())
	}

	// Читаем email
	fmt.Println((<-cResult).Email)

	// Ожидаем подтверждения (можно в отдельной горутине)
	if r := <-cResult; r == nil || r.Error == nil {
		fmt.Println("Почта подтверждена")
	}

}
