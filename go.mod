module linkcheck

go 1.25.1

require (
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.0.0
	github.com/alecthomas/kong v0.9.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/JohannesKaufmann/html-to-markdown/v2 => ./third_party/html-to-markdown
