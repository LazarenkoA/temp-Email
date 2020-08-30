package main

import (
	"fmt"
	postshift "github.com/LazarenkoA/post-shift-client"
	"regexp"
	"strings"
	"time"
)

func main() {
	reg := regexp.MustCompile(`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&\/=]*)`)
	factivation := func(from, body string) bool {
		m := reg.FindStringSubmatch(body)
		if len(m) > 0 {
			// getRequest(m[0]) // GET запрос
		}

		// Если функция возвращает true это значит что почта подтверждена и нам она больше не нужна.
		// После подтверждения или по таймауту (задается в настройках) временная почта удаляется т.к. можно получить ошибку exceeded_the_limit_for_one_person
		return strings.Contains(from, "admin@mail.ru") && len(m) > 0
	}

	cResult := make(chan *postshift.Result, 1) // размер канала обязательно должен быть 1 или больше
	newEmail := new(postshift.TmpEmail).Create(&postshift.TmpEmailConf {
		Result:     cResult,     // канал для результата
		Timeout:    time.Minute*2, // Таймаут в течении которого будет ожидаться письмо с подтверждением
		Activation: factivation, // функция для обработки входящих сообщений
	})

	if err := newEmail.NewRegistration(true); err != nil {
		fmt.Println("Произошла ошибка при регистрации почты ", err.Error())
	}

	// Читаем email
	fmt.Println((<-cResult).Email)


	// Ожидаем подтверждения (можно в отдельной горутине)
	if r := <-cResult; r.Error != nil && r.Confirm {
		fmt.Println("Почта подтверждена")
	}

}
