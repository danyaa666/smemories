// Package notefields is the closed catalogue of note form fields (decision D-21). A template can only
// ask for fields listed here, so every id has known limits, text rules and EN/VI labels. Ids are
// permanent: stored notes refer to them (see docs/note-fields.md).
package notefields

// Kind is the input type of a field.
type Kind string

const (
	ShortText Kind = "short_text" // one line, no newline
	LongText  Kind = "long_text"  // may contain "\n"
)

// Text is a string in both supported languages.
type Text struct {
	En string `json:"en"`
	Vi string `json:"vi"`
}

// Field describes one catalogue entry. Hint is the placeholder text of the form input.
type Field struct {
	ID        string
	Kind      Kind
	MaxLength int // in characters, after normalisation
	Label     Text
	Hint      Text
}

var fields = []Field{
	{"name", ShortText, 60,
		Text{"Your name", "Tên của bạn"},
		Text{"Your full name or the name they call you", "Họ tên của bạn hoặc tên mọi người hay gọi"}},
	{"nickname", ShortText, 40,
		Text{"Nickname", "Biệt danh"},
		Text{"What friends call you", "Tên bạn bè hay gọi bạn"}},
	{"relationship", ShortText, 60,
		Text{"Your relationship", "Mối quan hệ"},
		Text{"e.g. classmate, desk mate, teacher", "VD: bạn cùng lớp, bạn cùng bàn, thầy cô"}},
	{"message", LongText, 2000,
		Text{"Your message", "Lời nhắn"},
		Text{"Write something they will enjoy reading years from now", "Viết điều gì đó để bạn ấy đọc lại sau nhiều năm nữa"}},
	{"how_we_met", LongText, 500,
		Text{"How we met", "Chúng ta quen nhau thế nào"},
		Text{"Tell the story of how you became friends", "Kể lại chuyện hai bạn đã thân nhau như thế nào"}},
	{"first_impression", LongText, 500,
		Text{"First impression", "Ấn tượng đầu tiên"},
		Text{"What did you think when you first met?", "Lần đầu gặp, bạn nghĩ gì về bạn ấy?"}},
	{"best_memory", LongText, 500,
		Text{"Best memory together", "Kỷ niệm đẹp nhất của chúng ta"},
		Text{"A moment you will never forget", "Một khoảnh khắc bạn sẽ không bao giờ quên"}},
	{"wish", LongText, 500,
		Text{"My wish for you", "Điều mình ước cho bạn"},
		Text{"What do you hope for their future?", "Bạn mong tương lai của bạn ấy sẽ ra sao?"}},
	{"advice", LongText, 500,
		Text{"My advice", "Lời khuyên"},
		Text{"One piece of advice for what comes next", "Một lời khuyên cho chặng đường phía trước"}},
}

var byID = func() map[string]Field {
	m := make(map[string]Field, len(fields))
	for _, f := range fields {
		m[f.ID] = f
	}
	return m
}()

// FieldRef is a template's use of a catalogue field.
type FieldRef struct {
	ID       string
	Required bool
}

// Default is the field set of a yearbook whose template chooses none.
func Default() []FieldRef {
	return []FieldRef{{"name", true}, {"relationship", false}, {"message", true}}
}
