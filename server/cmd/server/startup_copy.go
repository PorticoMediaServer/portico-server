package main

import "golang.org/x/text/language"

// Recovery copy is shared with the catalogue and checked by the server tests.
var startupCopy = map[string]string{
	"server.startup.failed":      "Portico could not start",
	"server.startup.phase":       "The server could not finish its startup checks.",
	"server.startup.recovery":    "Check the service log for details. Fix the cause, then restart Portico. Your files have not been removed.",
	"server.startup.storage":     "Check that the state folder is available, has free space, and can be read and written by the service.",
	"server.startup.version":     "This database does not match this Portico version. Keep a copy of the state folder, then restore a backup made with this version or start with a new state folder.",
	"server.startup.permissions": "Give the service account permission to read and write the state folder, then restart Portico.",
	"server.startup.space":       "Free space on the drive holding the state folder, then restart Portico.",
	"server.startup.instance":    "Stop the other Portico service using this state folder, then restart this service.",
}

// The same copy in the catalogue's other languages (packages/i18n/translations/<locale>.json; the
// tests hold the two together). The page is read by whoever opens the server in a browser, so it
// follows their Accept-Language; the JSON reply keeps English, as every API message does.
var translatedStartupCopy = map[string]map[string]string{
	"fr": {
		"server.startup.failed":      "Portico n’a pas pu démarrer",
		"server.startup.phase":       "Le serveur n’a pas pu terminer ses vérifications de démarrage.",
		"server.startup.recovery":    "Consultez le journal du service pour plus de détails. Corrigez la cause, puis redémarrez Portico. Vos fichiers n’ont pas été supprimés.",
		"server.startup.storage":     "Vérifiez que le dossier d’état est disponible, dispose d’espace libre et est accessible en lecture et en écriture par le service.",
		"server.startup.version":     "Cette base de données ne correspond pas à cette version de Portico. Conservez une copie du dossier d’état, puis restaurez une sauvegarde faite avec cette version ou repartez d’un nouveau dossier d’état.",
		"server.startup.permissions": "Autorisez le compte du service à lire et écrire dans le dossier d’état, puis redémarrez Portico.",
		"server.startup.space":       "Libérez de l’espace sur le disque qui contient le dossier d’état, puis redémarrez Portico.",
		"server.startup.instance":    "Arrêtez l’autre service Portico qui utilise ce dossier d’état, puis redémarrez ce service.",
	},
	"es": {
		"server.startup.failed":      "Portico no ha podido iniciarse",
		"server.startup.phase":       "El servidor no ha podido completar sus comprobaciones de inicio.",
		"server.startup.recovery":    "Consulta el registro del servicio para ver los detalles. Corrige la causa y reinicia Portico. No se han eliminado tus archivos.",
		"server.startup.storage":     "Comprueba que la carpeta de estado esté disponible, tenga espacio libre y que el servicio pueda leerla y escribir en ella.",
		"server.startup.version":     "Esta base de datos no corresponde a esta versión de Portico. Guarda una copia de la carpeta de estado y después restaura una copia de seguridad hecha con esta versión o empieza con una carpeta de estado nueva.",
		"server.startup.permissions": "Da permiso a la cuenta del servicio para leer y escribir en la carpeta de estado y reinicia Portico.",
		"server.startup.space":       "Libera espacio en la unidad donde está la carpeta de estado y reinicia Portico.",
		"server.startup.instance":    "Detén el otro servicio de Portico que usa esta carpeta de estado y reinicia este servicio.",
	},
	"de": {
		"server.startup.failed":      "Portico konnte nicht starten",
		"server.startup.phase":       "Der Server konnte seine Startprüfungen nicht abschließen.",
		"server.startup.recovery":    "Details findest du im Dienstprotokoll. Behebe die Ursache und starte Portico dann neu. Deine Dateien wurden nicht entfernt.",
		"server.startup.storage":     "Prüfe, ob der Statusordner verfügbar ist, freien Platz hat und vom Dienst gelesen und beschrieben werden kann.",
		"server.startup.version":     "Diese Datenbank passt nicht zu dieser Portico-Version. Bewahre eine Kopie des Statusordners auf und stelle dann eine mit dieser Version erstellte Sicherung wieder her oder beginne mit einem neuen Statusordner.",
		"server.startup.permissions": "Gib dem Dienstkonto Lese- und Schreibrechte für den Statusordner und starte Portico dann neu.",
		"server.startup.space":       "Schaffe Platz auf dem Laufwerk mit dem Statusordner und starte Portico dann neu.",
		"server.startup.instance":    "Beende den anderen Portico-Dienst, der diesen Statusordner nutzt, und starte dann diesen Dienst neu.",
	},
	"ru": {
		"server.startup.failed":      "Не удалось запустить Portico",
		"server.startup.phase":       "Серверу не удалось завершить проверки при запуске.",
		"server.startup.recovery":    "Подробности смотрите в журнале службы. Устраните причину, затем перезапустите Portico. Ваши файлы не удалены.",
		"server.startup.storage":     "Убедитесь, что папка состояния доступна, на диске есть свободное место, а служба может читать и записывать эту папку.",
		"server.startup.version":     "Эта база данных не соответствует этой версии Portico. Сохраните копию папки состояния, затем восстановите резервную копию, созданную в этой версии, или начните с новой папки состояния.",
		"server.startup.permissions": "Дайте учётной записи службы права на чтение и запись папки состояния, затем перезапустите Portico.",
		"server.startup.space":       "Освободите место на диске с папкой состояния, затем перезапустите Portico.",
		"server.startup.instance":    "Остановите другую службу Portico, использующую эту папку состояния, затем перезапустите эту службу.",
	},
}

var startupLanguages = []string{"en", "fr", "es", "de", "ru"}
var startupMatcher = language.NewMatcher([]language.Tag{language.AmericanEnglish, language.French, language.Spanish, language.German, language.Russian})

// startupLanguage picks the page's language from an Accept-Language header; English when nothing matches.
func startupLanguage(acceptLanguage string) string {
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil || len(tags) == 0 {
		return "en"
	}
	_, index, confidence := startupMatcher.Match(tags...)
	if confidence == language.No {
		return "en"
	}
	return startupLanguages[index]
}

// startupText is one recovery message in a language, falling back to English.
func startupText(lang, id string) string {
	if text := translatedStartupCopy[lang][id]; text != "" {
		return text
	}
	return startupCopy[id]
}
