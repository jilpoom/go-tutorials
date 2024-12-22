# [Go Tutorial](https://go.dev/doc/tutorial/)

### What is GoLang?

> **Go 언어는 간결한 문법과 고루틴을 통한 동시성 처리에 강점이 있는 현대적 시스템 프로그래밍 언어다.**

Go는 Google에서 개발한 오픈소스 프로그래밍 언어로, 고성능 네트워킹 및 서버 애플리케이션 개발에 최적화되어 있습니다. 간단한 문법과 효율적인 병렬 처리를 제공하며, 크로스 플랫폼 컴파일과 독립 실행 파일 생성이 가능합니다.

### Where is GoLang used?

1. **웹 서버 개발**  
   - HTTP 서버, RESTful API, 웹 애플리케이션 개발
   - 예: Kubernetes의 일부 모듈, Docker CLI

2. **마이크로서비스**  
   - 가벼운 바이너리와 간결한 코드 구조로 마이크로서비스 아키텍처에 적합  
   - 예: Monzo, Uber의 마이크로서비스 

3. **클라우드 네이티브 애플리케이션**  
   - 클라우드 환경에서 동작하는 서비스 구축  
   - 예: Kubernetes, Terraform

4. **도구 및 CLI 개발**  
   - 독립 실행 가능한 경량 도구와 커맨드라인 인터페이스 개발  
   - 예: Hugo, Cobra

5. **네트워킹 및 분산 시스템**  
   - 고루틴과 채널을 활용한 고성능 네트워크 프로그래밍  
   - 예: NATS, Consul

6. **데이터 프로세싱 및 머신러닝** *(제한적)*  
   - 대규모 데이터 처리 파이프라인 구현  
   - 예: Pachyderm

---

### Why Use GoLang?

- **간결하고 쉬운 문법**: 빠르게 배우고 작성할 수 있는 언어.
- **동시성 처리**: 고루틴과 채널로 멀티스레드 코드를 간단히 작성.
- **빠른 컴파일**: 크고 복잡한 프로젝트도 짧은 시간 내에 빌드 가능.
- **강력한 표준 라이브러리**: 네트워킹, 암호화, 텍스트 처리 등 기본 제공.
- **크로스 플랫폼**: 다양한 OS에서 동일한 바이너리 생성.

---

### How to Get Started?

1. **Install Go**  
   [Go 설치 가이드](https://go.dev/doc/install)를 따라 설치하세요.

2. **Write Your First Program**
   ```go
   package main

   import "fmt"

   func main() {
       fmt.Println("Hello, World!")
   }
   ```

3. **Run the program**
    ```bash
    $ go run .
    ```

### Reference
- [Go Tutorial](https://go.dev/doc/tutorial/)
- [Awesome Go](https://awesome-go.com/#contents)
