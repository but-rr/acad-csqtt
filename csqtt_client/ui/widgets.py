"""Reusable native Tk widgets."""
import tkinter as tk
from tkinter import ttk
from PIL import ImageTk

from csqtt_client.ui.theme import BG, PANEL, SOFT, TEXT, MINT, MUTED, FIELD, rounded_surface, ui_font


class PowerButton(tk.Canvas):
    """Keyboard-accessible circular connection control with no layout animation."""
    def __init__(self, parent, command, scale=1):
        self.diameter = round(122 * scale)
        super().__init__(parent, width=self.diameter, height=self.diameter,
                         bg=parent.cget("bg"), highlightthickness=0, bd=0, takefocus=True, cursor="hand2")
        self.command, self.state, self.hover = command, "offline", False
        self.bind("<Button-1>", self.activate)
        self.bind("<Return>", self.activate)
        self.bind("<space>", self.activate)
        self.bind("<Enter>", lambda e: self.paint(hover=True))
        self.bind("<Leave>", lambda e: self.paint(hover=False))
        self.bind("<FocusIn>", lambda e: self.paint())
        self.bind("<FocusOut>", lambda e: self.paint())
        self.paint()

    def activate(self, event=None):
        if self.state != "stopping":
            self.focus_set()
            self.command()
        return "break"

    def paint(self, state=None, hover=None):
        if state is not None:
            self.state = state
        if hover is not None:
            self.hover = hover
        self.delete("all")
        d = self.diameter
        color = MINT if self.state == "online" else TEXT if self.hover else MUTED
        self.create_oval(3, 3, d-3, d-3, outline=MINT if self.focus_get() == self else SOFT, width=2)
        self.create_oval(11, 11, d-11, d-11, fill=FIELD if self.hover else PANEL, outline=SOFT, width=1)
        self.create_arc(d*.34, d*.31, d*.66, d*.63, start=135, extent=270, style="arc", outline=color, width=4)
        self.create_line(d*.5, d*.26, d*.5, d*.46, fill=color, width=4, capstyle="round")
        text = "Отключить" if self.state not in ("offline", "stopping") else "Подключить" if self.state == "offline" else "Отключение…"
        self.create_text(d*.5, d*.76, text=text, fill=color, font=ui_font(10, True))


def label(parent, text=None, variable=None, size=10, color=TEXT, bold=False, **kwargs):
    return tk.Label(parent, text=text, textvariable=variable, bg=parent.cget("bg"), fg=color,
                    font=ui_font(size, bold), anchor=kwargs.pop("anchor", "w"), **kwargs)


def frame(parent, bg=None, **kwargs):
    return tk.Frame(parent, bg=bg or parent.cget("bg"), **kwargs)


class Panel(tk.Canvas):
    def __init__(self, parent, padding=20, radius=18, stretch=False, fill=PANEL, outline=SOFT, **kwargs):
        super().__init__(parent, bg=parent.cget("bg"), highlightthickness=0, bd=0, **kwargs)
        self.padding = padding
        self.fill, self.outline = fill, outline
        self.radius = round(radius * max(1, self.winfo_fpixels("1i") / 96))
        self.stretch = stretch
        self.inset_y = max(padding, self.radius) if stretch else padding
        self.surface = None
        self.last_size = None
        self.body = tk.Frame(self, bg=fill)
        self.surface_id = self.create_image(0, 0, anchor="nw")
        self.window = self.create_window(padding, self.inset_y, window=self.body, anchor="nw")
        self.bind("<Configure>", self.resize)
        if not stretch:
            self.body.bind("<Configure>", lambda event: self.configure(height=self.body.winfo_reqheight() + 2 * padding))

    def resize(self, event):
        if event.width < 2 or event.height < 2 or self.last_size == (event.width, event.height):
            return
        self.last_size = (event.width, event.height)
        image = rounded_surface(event.width, event.height, self.radius, self.fill, self.outline, self.cget("bg"))
        self.surface = ImageTk.PhotoImage(image, master=self)
        self.itemconfigure(self.surface_id, image=self.surface)
        self.itemconfigure(self.window, width=max(1, event.width - 2 * self.padding))
        if self.stretch:
            self.itemconfigure(self.window, height=max(1, event.height - 2 * self.inset_y))


class ScrollPage(tk.Frame):
    def __init__(self, parent):
        super().__init__(parent, bg=BG)
        self.canvas = tk.Canvas(self, bg=BG, bd=0, highlightthickness=0)
        self.scroll = ttk.Scrollbar(self, orient="vertical", command=self.canvas.yview)
        self.canvas.configure(yscrollcommand=self.set_scroll)
        self.scroll.pack(side="right", fill="y")
        self.canvas.pack(side="left", fill="both", expand=True)
        self.body = tk.Frame(self.canvas, bg=BG)
        self.window = self.canvas.create_window(0, 0, window=self.body, anchor="nw")
        self.body.bind("<Configure>", lambda event: self.canvas.configure(scrollregion=self.canvas.bbox("all")))
        self.canvas.bind("<Configure>", lambda event: self.canvas.itemconfigure(self.window, width=event.width))

    def set_scroll(self, first, last):
        self.scroll.set(first, last)
        # Keep the gutter's width constant: toggling it during layout caused a
        # cascade of canvas resizes and full panel redraws on every page change.
        self.scroll.state(["disabled"] if float(first) <= 0 and float(last) >= 0.999 else ["!disabled"])
