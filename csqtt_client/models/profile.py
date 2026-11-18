"""Connection settings and validation, independent of the desktop UI."""
from dataclasses import dataclass
import ipaddress
import re


@dataclass
class Profile:
    server: str = ""
    password: str = ""
    hashes: str = ""
    proxy: str = "10.0.0.1:3128"
    proxy_user: str = ""
    proxy_password: str = ""
    socks_port: str = "1080"
    http_port: str = "8080"
    workers: str = "6"
    turn: str = ""
    obfs: str = "unified"
    srtp: str = "auto"
    generation: str = "0"
    salt: str = ""
    internet_mode: str = "proxy"
    system_proxy: str = "0"
    tun: str = "0"

    def validate(self):
        host, port = split_address(self.server)
        if not host or not 1 <= port <= 65535:
            raise ValueError("Сервер csqtt: укажите IP или домен и порт, например 203.0.113.10:443.")
        if self.internet_mode not in ("proxy", "direct") or self.system_proxy not in ("0", "1") or self.tun not in ("0", "1"):
            raise ValueError("Неверный режим подключения.")
        if self.internet_mode == "proxy":
            phost, pport = split_address(self.proxy)
            ipaddress.IPv4Address(phost)
            if not 1 <= pport <= 65535:
                raise ValueError("Проверьте порт корпоративного прокси.")
        if not self.password:
            raise ValueError("Введите пароль пользователя сервера csqtt.")
        hashes = normalize_hashes(self.hashes)
        if not 1 <= len(hashes) <= 6:
            raise ValueError("Нужно от 1 до 6 хешей VK или ссылок /call/join/.")
        if self.internet_mode == "proxy" and self.proxy_user and not self.proxy_password:
            raise ValueError("Введите пароль корпоративного прокси.")
        if self.internet_mode == "proxy" and (":" in self.proxy_user or any(c in self.proxy_user + self.proxy_password for c in "\r\n")):
            raise ValueError("Логин/пароль прокси содержат недопустимые символы.")
        if self.turn and (":" in self.turn or "/" in self.turn or any(c.isspace() for c in self.turn)):
            raise ValueError("TURN: только IPv4 или имя хоста без схемы и порта. Порт фиксирован: 443.")
        ports = [int(self.socks_port), int(self.http_port)]
        if any(not 1024 <= p <= 65535 for p in ports) or ports[0] == ports[1]:
            raise ValueError("Локальные порты: разные числа от 1024 до 65535.")
        if not 1 <= int(self.workers) <= 100:
            raise ValueError("Число потоков: от 1 до 100.")
        if not 0 <= int(self.generation) < 2**64:
            raise ValueError("Поколение: целое неотрицательное число.")
        if self.obfs not in ("unified", "audio", "video") or self.srtp not in ("auto", "wrap", "dtls"):
            raise ValueError("Неверный режим протокола.")
        return hashes


def split_address(value):
    host, port = value.strip().rsplit(":", 1)
    if any(c.isspace() for c in host) or any(c in host for c in "/@?#"):
        raise ValueError("Адрес должен иметь вид IP:порт или домен:порт.")
    return host.strip("[]"), int(port)


def normalize_hashes(value):
    result = []
    for token in re.split(r"[\s,;]+", value.strip()):
        token = token.strip("<>\"'")
        if "/call/join/" in token:
            token = token.split("/call/join/", 1)[1]
        elif token.startswith(("http://", "https://")):
            raise ValueError("Ссылка VK должна содержать /call/join/.")
        token = re.split(r"[?#/]", token, maxsplit=1)[0]
        if token and token not in result:
            result.append(token)
    return result
