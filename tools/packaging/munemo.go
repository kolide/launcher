package packaging

import (
	"bytes"
)

type munemo struct {
	negativeSymbol string
	symbols        []string
	buffer         *bytes.Buffer
}

func newMunemo() *munemo {
	return &munemo{
		symbols: []string{
			"ba", "bi", "bu", "be", "bo",
			"cha", "chi", "chu", "che", "cho",
			"da", "di", "du", "de", "do",
			"fa", "fi", "fu", "fe", "fo",
			"ga", "gi", "gu", "ge", "go",
			"ha", "hi", "hu", "he", "ho",
			"ja", "ji", "ju", "je", "jo",
			"ka", "ki", "ku", "ke", "ko",
			"la", "li", "lu", "le", "lo",
			"ma", "mi", "mu", "me", "mo",
			"na", "ni", "nu", "ne", "no",
			"pa", "pi", "pu", "pe", "po",
			"ra", "ri", "ru", "re", "ro",
			"sa", "si", "su", "se", "so",
			"sha", "shi", "shu", "she", "sho",
			"ta", "ti", "tu", "te", "to",
			"tsa", "tsi", "tsu", "tse", "tso",
			"wa", "wi", "wu", "we", "wo",
			"ya", "yi", "yu", "ye", "yo",
			"za", "zi", "zu", "ze", "zo",
		},
		negativeSymbol: "xa",
		buffer:         new(bytes.Buffer),
	}
}

func (m *munemo) String() string {
	return m.buffer.String()
}

func (m *munemo) Calculate(number int) {
	if number < 0 {
		m.buffer.Write([]byte(m.negativeSymbol))
		return
	}

	modulo := number % len(m.symbols)
	result := number / len(m.symbols)

	if result > 0 {
		m.Calculate(result)
	}

	m.buffer.Write([]byte(m.symbols[modulo]))
}

// Munemo is based off of the ruby library https://github.com/jmettraux/munemo.
// It provides a deterministic way to generate a string from a number.
func Munemo(id int) string {
	m := newMunemo()
	m.Calculate(id)
	return m.String()
}
