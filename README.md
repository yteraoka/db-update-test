# SELECT FOR UPDATE のテスト

## 概要

MySQL と PostgreSQL で顕著な違いがあるかどうかを確認する

次の Table に対して

```sql
CREATE TABLE sequences (id varchar(36), counter bigint, PRIMARY KEY (id));
```

こんなレコードを INSERT した状態で

```sql
INSERT INTO sequences (id, counter) VALUES ('ff99395b-2c70-42be-a667-ffe1cb8efe62', 0);
INSERT INTO sequences (id, counter) VALUES ('ffa08106-ec1e-40f8-8290-99c1cc6a4ef6', 0);
INSERT INTO sequences (id, counter) VALUES ('ffa24893-d8e9-4d73-a9c5-e80e9d5fd290', 0);
INSERT INTO sequences (id, counter) VALUES ('ffa51826-aa0e-4951-b8cc-2ea76a05394e', 0);
INSERT INTO sequences (id, counter) VALUES ('ffa8df8d-5461-4d58-8d1b-4c33b03ec258', 0);
```

SELECT FOR UPDATE を用いた更新を沢山実行する

```
BEGIN;
SELECT id, counter FROM sequences WHERE id = ? FOR UPDATE;
UPDATE sequences SET counter = ? + 1 WHERE id = ?;
COMMIT;
```

## Build

```
go mod tidy
go build
```

## 手順 (MySQL)

```
docker run \
  -d \
  --name mysql \
  -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=password \
  -e MYSQL_DATABASE=mydb \
  -e MYSQL_USER=user \
  -e MYSQL_PASSWORD=password \
  --rm \
  mysql:5.7
```

10,000 (init-records) レコード挿入したテーブルで、ランダムに選択したレコードの更新を
100,000 (update-count) 回 30 (workers) の go routine で分散して行う。

```
DSN="user:password@tcp(127.0.0.1:3306)/mydb" \
./db-update-test \
  -db-server=mysql \
  -update-count=100000 \
  -workers=30 \
  -max-connections=30 \
  -warn-threshold=500 \
  -init \
  -init-records=10000
```

MySQL は初回の最初だけ1秒くらいかかるのがぽつぽつ発生する。

```
docker stop mysql
```

実行例

```
% DSN="user:password@tcp(127.0.0.1:3306)/mydb" ./db-update-test -db-server=mysql -update-count=100000 -workers=30 -max-connections=30 -warn-threshold=500 -init -init-records=10000

2021/09/13 11:54:23 starting table initialize
2021/09/13 11:54:55 table initialized
Count: 100000
Min: 7 ms
Max: 471 ms
Ave: 114 ms
[histogram]
7-53.4       10.7%   █▍      10713
53.4-99.8    38.5%   █████▏  38532
99.8-146.2   22.5%   ███     22476
146.2-192.6  17.7%   ██▍     17730
192.6-239    7.66%   █       7656
239-285.4    2.1%    ▎       2095
285.4-331.8  0.554%  ▏       554
331.8-378.2  0.183%  ▏       183
378.2-424.6  0.049%  ▏       49
424.6-471    0.012%  ▏       12
```

## 手順 (PostgreSQL)

```
docker run \
  -d \
  --name postgres \
  -p 5432:5432 \
  -e POSTGRES_DB=mydb \
  -e POSTGRES_USER=user \
  -e POSTGRES_PASSWORD=password \
  --rm \
  postgres:13
```

```
DSN="user=user password=password host=127.0.0.1 port=5432 dbname=mydb sslmode=disable" \
./db-update-test \
  -db-server=postgres \
  -update-count=100000 \
  -workers=30 \
  -max-connections=30 \
  -warn-threshold=500 \
  -init \
  -init-records=10000
```

```
docker stop postgres
```
