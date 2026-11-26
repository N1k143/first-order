package api

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	_ "golang.org/x/image/webp"
)

const maxAvatarBytes = 256 * 1024

// External URLs are validated but never fetched by the API. Embedded images
// must match the declared format and decode within bounded bytes and pixels.
func validateAvatar(value *string) error {
	if value == nil {
		return nil
	}
	*value = strings.TrimSpace(*value)
	if *value == "" {
		return invalid("avatar_url", "Для удаления фото передайте null")
	}
	if !strings.HasPrefix(*value, "data:") {
		if len(*value) > 2048 {
			return invalid("avatar_url", "Ссылка на фото слишком длинная")
		}
		return validateURL("avatar_url", *value)
	}
	prefix, encoded, ok := strings.Cut(*value, ",")
	formats := map[string]string{"data:image/png;base64": "png", "data:image/jpeg;base64": "jpeg", "data:image/webp;base64": "webp"}
	format, allowed := formats[prefix]
	if !ok || !allowed || len(encoded) > base64.StdEncoding.EncodedLen(maxAvatarBytes) || strings.ContainsAny(encoded, "\r\n") {
		return invalid("avatar_url", "Выберите PNG, JPEG или WebP размером до 256 КБ")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > maxAvatarBytes {
		return invalid("avatar_url", "Некорректное изображение или размер больше 256 КБ")
	}
	config, actual, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || actual != format {
		return invalid("avatar_url", "Содержимое фото не соответствует формату")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 {
		return invalid("avatar_url", "Размер фото не должен превышать 2048 × 2048 пикселей")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return invalid("avatar_url", "Не удалось прочитать изображение")
	}
	return nil
}
