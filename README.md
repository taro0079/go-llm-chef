# go-llm-chef

PCでRaspberry Pi向けのDockerイメージをビルドしてGHCRにpushし、Piではpullして起動します。PiにGoやソースコードは不要です。

## PCでビルド・push

Docker Desktop（Buildxを含む）を起動してください。標準の対象は64bit Raspberry Pi OS（`linux/arm64`）です。Piで `uname -m` を実行して、`aarch64` ならこの設定を使います。

初回はGitHubのPersonal access token **(classic)** に `write:packages` を付け、GHCRにログインします。以下のPassword入力にはそのトークンを入力します。

```sh
docker login ghcr.io -u taro0079
```

プロジェクトのルートで実行します。

```sh
# 64bit Pi向けにビルドし、latestタグでpush
sh scripts/publish-image.sh

# バージョンを指定する場合（この場合はlatestを更新しません）
sh scripts/publish-image.sh v1.0.0
```

`github.com/mattn/go-sqlite3` がCGOを必要とするため、Dockerの対象プラットフォーム上でCコンパイラを使ってビルドします。Docker Desktopのエミュレーションで異なるCPU向けにもビルドできますが、初回は時間がかかる場合があります。

32bit OS（`uname -m` が `armv7l`）向けや、両方のOS向けに配布する場合は指定を切り替えます。ARMv6のPiには対応していません。

```sh
PLATFORMS=linux/arm/v7 sh scripts/publish-image.sh
PLATFORMS=linux/arm64,linux/arm/v7 sh scripts/publish-image.sh
```

別のGHCR名前空間を使う場合は `IMAGE_NAME=ghcr.io/your-account/go-llm-chef` を付けて実行し、Pi側の `.env` も合わせてください。スクリプトはPCの `.env` を読み込みません。

## Raspberry Piで起動

Docker EngineとComposeプラグインをインストール済みのPiに、`compose.yaml` と `.env.example` だけを同じディレクトリへコピーします。例（`pi@raspberrypi.local` は自分の接続先に置き換えます）:

```sh
# PC上で実行
ssh pi@raspberrypi.local 'mkdir -p ~/go-llm-chef'
scp compose.yaml .env.example pi@raspberrypi.local:~/go-llm-chef/
```

Pi上で設定します。

```sh
cd ~/go-llm-chef
cp .env.example .env
chmod 600 .env
nano .env
```

`.env` に `OPENAI_API_KEY` を設定してください。バージョン付きでpushした場合は `IMAGE_TAG=v1.0.0` のように合わせます。

GHCRのパッケージがprivateの場合、Piでも `read:packages` を持つPAT (classic) でログインします。

```sh
docker login ghcr.io -u taro0079
```

初回pushで作成されたパッケージはprivateです。公開してよい場合はGitHubのパッケージ設定でpublicにすると、Piでのログインが不要になります。

```sh
docker compose pull
docker compose up -d --no-build
docker compose logs --tail=100 app
```

ブラウザで `http://<PiのIPアドレス>:8080` にアクセスします。Composeに `build` は定義していないため、Piではビルドしません。複数プラットフォームのイメージをpushした場合は、PiのOSに対応するものをDockerが選択します。

## 更新

PCで `sh scripts/publish-image.sh` を再実行し、Piで以下を実行します。

```sh
cd ~/go-llm-chef
docker compose pull
docker compose up -d --no-build
```

バージョン付きタグを使う場合は、Piの `.env` の `IMAGE_TAG` を新しいタグに変更してから実行します。以前のタグに戻せばイメージも戻せます（DBは巻き戻りません）。

SQLiteの `app.db` は `app-data` ボリュームに永続化されます。同じディレクトリで更新を実行してください。`docker compose down -v` はDBも削除します。

参考: [Dockerのマルチプラットフォームビルド](https://docs.docker.com/build/building/multi-platform/)、[GHCRの認証と公開設定](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)。
