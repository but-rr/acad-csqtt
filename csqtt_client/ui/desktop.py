"""Desktop pages, navigation, and connection feedback."""
import ctypes
import tkinter as tk
from tkinter import ttk
from PIL import Image, ImageDraw, ImageTk

from csqtt_client.ui.theme import (
    BG, PANEL, FIELD, DEEP, TEXT, MUTED, DIM, LINE, SOFT, MINT, AMBER, RED, HEADER, ui_font,
)
from csqtt_client.ui.styles import configure_style
from csqtt_client.ui.widgets import Panel, ScrollPage, PowerButton, label, frame


class DesktopUI:
    def __init__(self, app, root, profile, version, directory):
        self.app, self.root = app, root
        self.app_version = version
        self.page = None
        self.images = []
        self.entries = {}
        self.secret_buttons = {}
        self.scale = max(1, root.winfo_fpixels("1i") / 96)
        if hasattr(ctypes, "windll"):
            for path in (directory / "assets/fonts").glob("*.ttf"):
                ctypes.windll.gdi32.AddFontResourceExW(str(path), 0x10, None)
        configure_style(root, self.scale, self.images)
        with Image.open(directory / "assets/icons/CS_logo.png") as source:
            self.logo = source.convert("RGBA")
        self.window_icon = ImageTk.PhotoImage(self.logo.resize((256, 256), Image.Resampling.LANCZOS), master=root)
        root.iconphoto(True, self.window_icon)
        root.iconbitmap(default=str(directory / "assets/icons/CSQTT.ico"))
        root.configure(bg=BG)
        root.title(f"CSQTT v{version}")
        scale = max(1, root.winfo_fpixels("1i") / 96)
        self.scale = scale
        width = min(int(1120 * scale), root.winfo_screenwidth()-80)
        height = min(int(900 * scale), root.winfo_screenheight()-100)
        root.geometry(f"{width}x{height}")
        root.minsize(min(980, width), min(700, height))
        from csqtt_client.platform.windows.chrome import WindowChrome
        self.chrome = WindowChrome(root, scale)
        self.title_bar(version)

        sidebar = Panel(root, padding=9, radius=24, stretch=True, width=max(86, int(68 * scale)))
        sidebar.pack(side="left", fill="y", padx=(18, 0), pady=18)
        sidebar.pack_propagate(False)
        rail = sidebar.body
        logo_size = round(38*self.scale)
        self.rail_logo = ImageTk.PhotoImage(self.logo.resize((logo_size, logo_size), Image.Resampling.LANCZOS), master=root)
        tk.Label(rail, image=self.rail_logo, bg=PANEL).pack(fill="x", pady=(8, 18))
        self.nav = {}
        self.nav_images = {}
        for key, text, icon in (("connection", "Туннель", "connection"),
                                ("settings", "Настройки", "settings"), ("logs", "Журнал", "logs")):
            self.nav_images[key] = (self.icon(icon), self.icon(icon, "#25262b"))
            button = ttk.Button(rail, text=text, image=self.nav_images[key][0], compound="none",
                                style="Nav.TButton", command=lambda k=key: self.show_page(k), takefocus=True)
            button.pack(pady=6)
            self.tooltip(button, text)
            self.nav[key] = button
        label(rail, f"v{version}", size=8, color=DIM, anchor="center").pack(side="bottom", fill="x", pady=(8, 2))
        label(rail, "TCP / 443", size=8, color=MUTED, anchor="center").pack(side="bottom", fill="x")

        main = frame(root)
        main.pack(side="left", fill="both", expand=True, padx=(24, 24), pady=(24, 18))
        header = frame(main)
        header.pack(fill="x", pady=(0, 18))
        titles = frame(header)
        titles.pack(side="left", fill="x", expand=True)
        self.heading = label(titles, "Подключение", size=21, bold=True)
        self.heading.pack(anchor="w")
        self.subtitle = label(titles, "Интернет через ваш сервер csqtt и корпоративный прокси.", color=MUTED)
        self.subtitle.pack(anchor="w", pady=(5, 0))
        self.badge = label(header, "●  Отключено", color=DIM, size=10)
        self.badge.pack(side="right", padx=(12, 0))

        self.update_banner = Panel(main, padding=10, radius=12)
        self.update_message = label(self.update_banner.body, "", color=MINT, size=10)
        self.update_message.pack(side="left", padx=5, pady=6)
        ttk.Button(self.update_banner.body, text="Обновить", command=app.download_update).pack(side="right")

        stats = frame(main)
        stats.pack(fill="x", pady=(0, 16))
        for column, (name, title, unit) in enumerate((("active_count", "Активные потоки", ""),
                  ("upload_total", "Отправлено", "МБ"), ("download_total", "Получено", "МБ"))):
            stats.columnconfigure(column, weight=1, uniform="metric")
            panel = Panel(stats, padding=14)
            panel.grid(row=0, column=column, sticky="ew", padx=(0, 12 if column < 2 else 0))
            label(panel.body, title, size=9, color=MUTED).pack(anchor="w")
            value = frame(panel.body)
            value.pack(fill="x", pady=(4, 0))
            variable = tk.StringVar(value="0" if not unit else "0.00")
            setattr(app, name, variable)
            label(value, variable=variable, size=21, bold=True).pack(side="left")
            if unit:
                label(value, unit, size=9, color=DIM).pack(side="left", padx=(8, 0), pady=(10, 0))
        compact = frame(main)
        summary = tk.StringVar()
        label(compact, variable=summary, size=9, color=MUTED).pack(anchor="w")
        def update_summary(*args):
            summary.set(f"Потоки: {app.active_count.get()}     ↑ {app.upload_total.get()} МБ     ↓ {app.download_total.get()} МБ")
        for variable in (app.active_count, app.upload_total, app.download_total):
            variable.trace_add("write", update_summary)
        update_summary()

        # Controls and connection state remain visible when changing pages.
        footer = frame(main)
        footer.pack(side="bottom", fill="x", pady=(16, 0))
        tk.Frame(footer, height=1, bg=SOFT).pack(fill="x", pady=(0, 12))
        self.status_label = label(footer, variable=app.status, color=MUTED, justify="left", wraplength=900)
        self.status_label.pack(fill="x", pady=(0, 12))
        self.status_label.bind("<Configure>", lambda e: self.status_label.configure(wraplength=max(200, e.width)))
        actions = frame(footer)
        actions.pack(fill="x")
        app.start_button = ttk.Button(actions, text="Подключить", style="Primary.TButton", command=app.start)
        app.start_button.pack(side="left")
        app.stop_button = ttk.Button(actions, text="Отключить", style="Footer.TButton", command=app.stop, state="disabled")
        app.stop_button.pack(side="left", padx=10)
        app.start_button.pack_forget()
        app.stop_button.pack_forget()
        ttk.Button(actions, text="Настройки подключения", command=lambda: self.show_page("settings")).pack(side="left")
        app.test_button = ttk.Button(actions, text="Проверить интернет", style="Footer.TButton", command=app.check_internet, state="disabled")
        app.test_button.pack(side="right")

        self.pages = frame(main)
        self.pages.pack(fill="both", expand=True)
        self.pages.rowconfigure(0, weight=1)
        self.pages.columnconfigure(0, weight=1)
        self.connection_page(profile)
        self.configuration_page(profile)
        self.logs_page()
        self.show_page("connection")
        previous_compact = None
        def resize_window(event):
            nonlocal previous_compact
            if event.widget is not root:
                return
            small = event.height / self.scale < 720
            if small == previous_compact:
                return
            previous_compact = small
            if small:
                stats.pack_forget()
                compact.pack(fill="x", pady=(0, 14), before=self.pages)
            else:
                compact.pack_forget()
                stats.pack(fill="x", pady=(0, 16), before=self.pages)
        root.bind("<Configure>", resize_window, add="+")
        root.bind("<MouseWheel>", self.scroll_page)

    def title_bar(self, version):
        bar = tk.Frame(self.root, bg=HEADER, height=round(46*self.scale))
        bar.pack(side="top", fill="x")
        bar.pack_propagate(False)
        tk.Frame(self.root, height=1, bg=SOFT).pack(side="top", fill="x")
        logo_size = round(24*self.scale)
        self.caption_logo = ImageTk.PhotoImage(self.logo.resize((logo_size, logo_size), Image.Resampling.LANCZOS), master=self.root)
        logo = tk.Label(bar, image=self.caption_logo, bg=HEADER)
        logo.pack(side="left", padx=(18, 8))
        brand = label(bar, "CSQTT", size=12, bold=True)
        brand.pack(side="left", padx=(0, 12))
        version_label = label(bar, f"v{version}", size=8, color=DIM)
        version_label.pack(side="left")
        drag_space = frame(bar)
        drag_space.pack(side="left", fill="both", expand=True)
        for widget in (bar, logo, brand, version_label, drag_space):
            widget.bind("<ButtonPress-1>", self.chrome.drag)
            widget.bind("<B1-Motion>", self.chrome.drag_motion)
            widget.bind("<ButtonRelease-1>", self.chrome.drag_end)
            widget.bind("<Double-Button-1>", self.chrome.maximize)
        controls = frame(bar)
        controls.pack(side="right", fill="y", padx=8, pady=7)
        for text, hint, command, style in (("—", "Свернуть в трей", self.app.hide_to_tray, "Chrome.TButton"),
                      ("□", "Развернуть / восстановить", self.chrome.maximize, "Chrome.TButton"),
                      ("×", "Скрыть в трей", self.app.hide_to_tray, "Close.Chrome.TButton")):
            button = ttk.Button(controls, text=text, width=2, command=command, style=style)
            button.pack(side="left", fill="y", padx=2)
            self.tooltip(button, hint)
            if text == "—":
                self.minimize_button = button
            elif text == "□":
                self.maximize_button = button
            else:
                self.hide_button = button
        self.titlebar = bar

    def tooltip(self, widget, text):
        pending, popup = None, None
        def hide(event=None):
            nonlocal pending, popup
            if pending:
                self.root.after_cancel(pending)
                pending = None
            if popup:
                popup.destroy()
                popup = None
        def show():
            nonlocal popup, pending
            pending = None
            popup = tk.Toplevel(self.root)
            popup.overrideredirect(True)
            popup.configure(bg=LINE)
            tk.Label(popup, text=text, bg=FIELD, fg=TEXT, font=ui_font(9), padx=10, pady=5).pack(padx=1, pady=1)
            x = min(widget.winfo_rootx()+widget.winfo_width()+7, self.root.winfo_screenwidth()-240)
            popup.geometry(f"+{x}+{widget.winfo_rooty()+widget.winfo_height()+4}")
        def enter(event):
            nonlocal pending
            hide()
            pending = self.root.after(450, show)
        for event in ("<Enter>", "<FocusIn>"):
            widget.bind(event, enter, add="+")
        for event in ("<Leave>", "<FocusOut>", "<ButtonPress-1>"):
            widget.bind(event, hide, add="+")



    def icon(self, name, color=MUTED):
        image = Image.new("RGBA", (48, 48))
        draw = ImageDraw.Draw(image)
        if name == "connection":
            draw.rounded_rectangle((8, 8, 39, 31), radius=3, outline=color, width=3)
            draw.line((24, 32, 24, 40), fill=color, width=3)
            draw.line((15, 40, 33, 40), fill=color, width=3)
        elif name == "settings":
            for y, x in ((16, 17), (33, 31)):
                draw.line((7, y, 41, y), fill=color, width=3)
                draw.line((x, y-7, x, y+7), fill=color, width=4)
        else:
            draw.rounded_rectangle((10, 6, 38, 42), radius=3, outline=color, width=3)
            for y in (16, 24, 32):
                draw.line((17, y, 31, y), fill=color, width=2)
        photo = ImageTk.PhotoImage(image.resize((24, 24), Image.Resampling.LANCZOS), master=self.root)
        self.images.append(photo)
        return photo

    def page_frame(self, key):
        page = ScrollPage(self.pages)
        page.grid(row=0, column=0, sticky="nsew")
        setattr(self, key, page)
        return page.body

    def section(self, parent, title, description=None):
        panel = Panel(parent)
        label(panel.body, title, size=11, bold=True).pack(anchor="w", pady=(0, 4))
        if description:
            hint = label(panel.body, description, color=MUTED, size=9, wraplength=520, justify="left")
            hint.pack(fill="x", pady=(0, 10))
            hint.bind("<Configure>", lambda e: hint.configure(wraplength=max(100, e.width)))
        return panel

    def field(self, parent, title, key, secret=False):
        group = frame(parent)
        group.pack(fill="x", pady=(10, 0))
        label(group, title, color=MUTED, size=9).pack(anchor="w", pady=(0, 5))
        row = frame(group)
        row.pack(fill="x")
        entry = ttk.Entry(row, width=1, textvariable=self.app.vars[key], show="•" if secret else "")
        self.entries[key] = entry
        entry.pack(side="left", fill="x", expand=True)
        if secret:
            def toggle():
                hidden = bool(entry.cget("show"))
                entry.configure(show="" if hidden else "•")
                button.configure(text="Скрыть" if hidden else "Показать")
            button = ttk.Button(row, text="Показать", command=toggle, width=8)
            button.pack(side="right", padx=(7, 0))
            self.secret_buttons[key] = button
        return entry

    def text(self, parent, height, expand=False, **kwargs):
        surface = Panel(parent, padding=12, radius=10, stretch=expand, fill=DEEP, outline=LINE)
        widget = tk.Text(surface.body, height=height, width=1, bg=DEEP, fg=TEXT, insertbackground=TEXT,
                         selectbackground="#43564c", selectforeground=TEXT, highlightthickness=0,
                         relief="flat", bd=0, padx=0, pady=0, font=("Consolas", 10), wrap="word", **kwargs)
        widget.pack(fill="both", expand=True)
        widget.surface = surface
        return widget

    def connection_page(self, profile):
        body = self.page_frame("connection")
        surface = Panel(body, padding=18, radius=24)
        surface.pack(fill="x")
        self.power = PowerButton(surface.body, self.app.toggle_connection, self.scale)
        self.power.pack(pady=(0, 10))
        self.connection_hint = label(surface.body, "Выберите режим и нажмите «Подключить»", size=9, color=MUTED, anchor="center")
        modes = frame(surface.body)
        modes.pack(anchor="center")
        self.mode_controls = []
        for text, key in (("Системный прокси", "system_proxy"), ("TUN · весь компьютер", "tun")):
            control = ttk.Checkbutton(modes, text=text, variable=self.app.vars[key], onvalue="1", offvalue="0", style="Panel.TCheckbutton",
                                      command=self.app.system_proxy_changed if key == "system_proxy" else None)
            control.pack(side="left", padx=10)
            if key == "tun": self.mode_controls.append(control)
            self.tooltip(control, "Применяется после подключения. При отключении восстанавливаются прежние параметры Windows."
                         if key == "system_proxy" else "Для сетевого адаптера Windows запросит права администратора. IPv6 блокируется на время TUN.")
        tk.Frame(surface.body, height=1, bg=SOFT).pack(fill="x", pady=12)
        self.corporate_switch = ttk.Checkbutton(surface.body, text="Корпоративный прокси",
            variable=self.app.vars["internet_mode"], onvalue="proxy", offvalue="direct", style="Switch.TCheckbutton")
        self.corporate_switch.pack(anchor="center")
        self.mode_controls.append(self.corporate_switch)
        self.internet_hint = label(surface.body, "", size=9, color=MUTED, anchor="center")
        self.internet_hint.pack(fill="x", pady=(5, 0))
        def changed(*args):
            corp = self.app.vars["internet_mode"].get() == "proxy"
            self.internet_hint.configure(text="Через HTTP CONNECT · TCP / 443" if corp else "Прямое подключение · Wi-Fi или обычный интернет")
        self.app.vars["internet_mode"].trace_add("write", changed)
        changed()

    def configuration_page(self, profile):
        body = self.page_frame("settings")
        body.columnconfigure(0, weight=3, uniform="connection")
        body.columnconfigure(1, weight=2, uniform="connection")
        server = self.section(body, "Сервер csqtt", "Данные подключения с вашего сервера.")
        server.grid(row=0, column=0, sticky="new", padx=(0, 14))
        self.field(server.body, "Сервер · IP или домен и порт", "server")
        self.field(server.body, "Пароль csqtt", "password", secret=True)
        label(server.body, "Хеши VK или ссылки звонков", color=MUTED, size=9).pack(anchor="w", pady=(14, 5))
        self.app.hashes = self.text(server.body, height=4)
        self.app.hashes.surface.pack(fill="x")
        self.app.hashes.insert("1.0", profile.hashes)
        label(server.body, "По одному на строку · до 6 хешей", color=DIM, size=8).pack(anchor="w", pady=(7, 2))
        side = frame(body)
        side.grid(row=0, column=1, sticky="new")
        proxy = self.section(side, "Корпоративный прокси", "HTTP CONNECT · авторизация Basic")
        proxy.pack(fill="x")
        self.field(proxy.body, "Адрес прокси · IP:порт", "proxy")
        self.field(proxy.body, "Логин", "proxy_user")
        self.field(proxy.body, "Пароль прокси", "proxy_password", secret=True)
        self.app.check_proxy_button = ttk.Button(proxy.body, text="Проверить прокси :443", command=self.app.check_proxy)
        self.app.check_proxy_button.pack(anchor="w", pady=(14, 0))
        self.proxy_feedback = label(proxy.body, "Прокси ещё не проверен", size=9, color=DIM,
                                    wraplength=360, justify="left")
        self.proxy_feedback.pack(fill="x", pady=(10, 0))
        self.proxy_feedback.bind("<Configure>", lambda e: self.proxy_feedback.configure(wraplength=max(120, e.width)))
        local = self.section(side, "Локальные прокси")
        local.pack(fill="x", pady=(14, 0))
        self.endpoints = {}
        for kind, key in (("SOCKS5", "socks_port"), ("HTTP", "http_port")):
            row = frame(local.body)
            row.pack(fill="x", pady=(8, 0))
            text = tk.StringVar()
            self.endpoints[key] = text
            label(row, variable=text, size=9, color=MUTED).pack(side="left")
            ttk.Button(row, text="Копировать", command=lambda k=key, t=kind: self.copy_endpoint(k, t)).pack(side="right")
            self.app.vars[key].trace_add("write", lambda *args: self.update_endpoints())
        self.update_endpoints()
        remember = frame(body)
        remember.grid(row=1, column=0, columnspan=2, sticky="w", pady=(12, 0))
        ttk.Checkbutton(remember, text="Сохранять настройки рядом с программой", variable=self.app.remember).pack(anchor="w")
        self.profile_feedback = label(remember, "Пароли и хеши сохраняются в profile.json открытым текстом.", size=8, color=DIM)
        self.profile_feedback.pack(anchor="w", padx=4, pady=(3, 0))
        self.adapt_columns(body, server, side, "connection", remember)
        self.advanced_settings(body)

    def advanced_settings(self, parent):
        body = frame(parent)
        body.grid(row=3, column=0, columnspan=2, sticky="ew", pady=(14, 0))
        body.columnconfigure(0, weight=1, uniform="settings")
        body.columnconfigure(1, weight=1, uniform="settings")
        transport = self.section(body, "Параметры туннеля", "Режимы должны соответствовать настройкам сервера.")
        transport.grid(row=0, column=0, sticky="new", padx=(0, 14))
        self.field(transport.body, "Потоки · workers", "workers")
        for title, key, values in (("Обфускация", "obfs", ("unified", "audio", "video")),
                                   ("SRTP", "srtp", ("auto", "wrap", "dtls"))):
            label(transport.body, title, size=9, color=MUTED).pack(anchor="w", pady=(14, 5))
            ttk.Combobox(transport.body, textvariable=self.app.vars[key], values=values, state="readonly").pack(fill="x")
        self.field(transport.body, "TURN-хост · необязательно", "turn")
        right = frame(body)
        right.grid(row=0, column=1, sticky="new")
        ports = self.section(right, "Локальные порты", "Доступны только на этом компьютере: 127.0.0.1.")
        ports.pack(fill="x")
        self.field(ports.body, "SOCKS5", "socks_port")
        self.field(ports.body, "HTTP", "http_port")
        session = self.section(right, "Параметры сессии", "Заполняйте, только если они выданы сервером.")
        session.pack(fill="x", pady=(14, 0))
        self.field(session.body, "Поколение", "generation")
        self.field(session.body, "Salt", "salt", secret=True)
        updates = self.section(right, "Обновления")
        updates.pack(fill="x", pady=(14, 0))
        self.update_feedback = label(updates.body, f"Установлена версия v{self.app_version}", size=9, color=MUTED,
                                     wraplength=420, justify="left")
        self.update_feedback.pack(fill="x", pady=(4, 10))
        self.update_feedback.bind("<Configure>", lambda e: self.update_feedback.configure(wraplength=max(150, e.width)))
        self.update_check_button = ttk.Button(updates.body, text="Проверить обновления", command=self.app.check_updates)
        self.update_check_button.pack(anchor="w")
        self.adapt_columns(body, transport, right, "settings")

    def show_update(self, version):
        self.update_message.configure(text=f"Доступна новая версия v{version}")
        self.update_feedback.configure(text=f"Доступна версия v{version}. Кнопка «Обновить» откроет скачивание ZIP.")
        self.update_banner.pack(fill="x", pady=(0, 14), before=self.pages)

    def adapt_columns(self, body, left, right, group, remember=None):
        previous = None
        def adapt(event):
            nonlocal previous
            stacked = event.width / self.scale < 760
            if stacked == previous:
                return
            previous = stacked
            body.columnconfigure(0, weight=1 if stacked else (3 if group == "connection" else 1), uniform="" if stacked else group)
            body.columnconfigure(1, weight=0 if stacked else (2 if group == "connection" else 1), uniform="" if stacked else group)
            left.grid_configure(padx=0 if stacked else (0, 14))
            right.grid_configure(row=1 if stacked else 0, column=0 if stacked else 1, pady=(14, 0) if stacked else 0)
            if remember:
                remember.grid_configure(row=2 if stacked else 1, columnspan=1 if stacked else 2)
        body.bind("<Configure>", adapt, add="+")

    def logs_page(self):
        body = frame(self.pages)
        body.grid(row=0, column=0, sticky="nsew")
        self.logs = body
        tools = frame(body)
        tools.pack(fill="x", pady=(0, 12))
        label(tools, "События подключения", size=12, bold=True).pack(side="left")
        self.app.copy_log_button = ttk.Button(tools, text="Скопировать журнал", command=self.app.copy_log)
        self.app.copy_log_button.pack(side="right")
        ttk.Button(tools, text="Сохранить…", command=self.app.save_log).pack(side="right", padx=10)
        self.app.log = self.text(body, height=10, expand=True, state="disabled")
        scrollbar = ttk.Scrollbar(body, orient="vertical", command=self.app.log.yview)
        scrollbar.pack(side="right", fill="y")
        self.app.log.configure(yscrollcommand=scrollbar.set)
        self.app.log.surface.pack(fill="both", expand=True)
        self.log_empty = label(self.app.log, "Журнал пока пуст\n\nЗдесь появятся события после подключения.",
                               color=DIM, size=11, anchor="center", justify="center")
        self.log_empty.place(relx=0.5, rely=0.45, anchor="center")
        self.app.log.tag_configure("error", foreground=RED)
        self.app.log.tag_configure("retry", foreground=AMBER)
        self.app.log.tag_configure("ready", foreground=MINT)
        label(body, "Пароли и хеши скрываются.   Ctrl+A — выделить всё   ·   Ctrl+C — копировать", size=9, color=DIM).pack(anchor="w", pady=(9, 0))

    def show_page(self, key):
        if key == self.page:
            return
        self.page = key
        for name in ("connection", "settings", "logs"):
            page = getattr(self, name)
            if not hasattr(page, "focus_targets"):
                page.focus_targets = []
                def collect(widget):
                    if isinstance(widget, (ttk.Entry, ttk.Button, ttk.Checkbutton, tk.Text, PowerButton)):
                        page.focus_targets.append((widget, widget.cget("takefocus")))
                    for child in widget.winfo_children():
                        collect(child)
                collect(page)
                for widget, original in page.focus_targets:
                    def can_focus(window, name=name, widget=widget, original=original):
                        if self.page != name or str(original) == "0":
                            return False
                        return not isinstance(widget, ttk.Widget) or not widget.instate(["disabled"])
                    widget.configure(takefocus=can_focus)
        focused = self.root.focus_get()
        if focused and str(focused).startswith(str(self.pages)) and not str(focused).startswith(str(getattr(self, key))):
            self.nav[key].focus_set()
        titles = {"connection": ("Подключение", "Интернет через ваш сервер CSQTT."),
                  "settings": ("Настройки", "Сервер, корпоративный прокси и параметры туннеля."),
                  "logs": ("Журнал", "События ядра, состояние туннеля и диагностика.")}
        title, subtitle = titles[key]
        self.heading.configure(text=title)
        self.subtitle.configure(text=subtitle)
        for name, button in self.nav.items():
            button.configure(style="Selected.Nav.TButton" if name == key else "Nav.TButton",
                             image=self.nav_images[name][1 if name == key else 0])
        getattr(self, key).tkraise()

    def scroll_page(self, event):
        if self.page != "logs" and not isinstance(event.widget, tk.Text):
            canvas = getattr(self, self.page).canvas
            if canvas.yview() != (0.0, 1.0):
                canvas.yview_scroll(-int(event.delta / 120), "units")

    def set_state(self, state):
        text, color = {"offline": ("Отключено", DIM), "connecting": ("Подключение", AMBER),
                       "checking": ("Проверка интернета", AMBER), "online": ("Подключено", MINT),
                       "unconfirmed": ("Интернет не подтверждён", AMBER),
                       "recovering": ("Восстановление", AMBER), "stopping": ("Отключение", DIM)}[state]
        self.badge.configure(text=f"●  {text}", fg=color)
        self.power.paint(state=state)
        self.connection_hint.configure(text=text, fg=color)
        for control in self.mode_controls:
            control.configure(state="normal" if state == "offline" else "disabled")
        self.status_label.configure(fg=color if state in ("online", "unconfirmed", "recovering") else MUTED)
        self.update_endpoints()

    def show_proxy_result(self, state, message):
        color = {"checking": AMBER, "success": MINT, "error": RED}[state]
        self.proxy_feedback.configure(text=message, fg=color)
        self.root.after_idle(self.reveal_proxy_result)

    def reveal_proxy_result(self):
        if self.page != "settings" or not self.proxy_feedback.winfo_ismapped():
            return
        page = self.settings
        bottom = self.proxy_feedback.winfo_rooty() - page.body.winfo_rooty() + self.proxy_feedback.winfo_height() + 10
        visible_bottom = page.canvas.canvasy(0) + page.canvas.winfo_height()
        if bottom > visible_bottom:
            page.canvas.yview_moveto(max(0, bottom - page.canvas.winfo_height()) / max(1, page.body.winfo_height()))

    def update_endpoints(self):
        for key, variable in self.endpoints.items():
            profile = self.app.running_profile if self.app.helper else None
            port = getattr(profile, key) if profile else self.app.vars[key].get()
            kind = "SOCKS5" if key == "socks_port" else "HTTP"
            variable.set(f"{kind}   127.0.0.1:{port}")

    def copy_endpoint(self, key, kind):
        profile = self.app.running_profile if self.app.helper else None
        port = getattr(profile, key) if profile else self.app.vars[key].get()
        self.root.clipboard_clear()
        self.root.clipboard_append(f"{'socks5h' if kind == 'SOCKS5' else 'http'}://127.0.0.1:{port}")
