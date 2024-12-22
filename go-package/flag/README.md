# [Flag](https://pkg.go.dev/flag)

- 명령줄 옵션 제어 패키지

## Usage

```go
var (
    name  = flag.String("name", "World", "이름")
    age   = flag.Int("age", 0, "나이")
    debug = flag.Bool("debug", false, "디버그 모드 활성화")
)

func main() {
    // 플래그 파싱
    flag.Parse()

    // 플래그 값 사용
    if *debug {
        log.Println("디버그 모드가 활성화되었습니다")
    }
    
    fmt.Printf("안녕하세요, %s! 당신은 %d살입니다.\n", *name, *age)
}
```