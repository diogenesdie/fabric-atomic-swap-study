"""
Identidade visual do seminário, extraída da apresentação original.

Mantemos o deck reproduzível: em vez de editar slides à mão, o conteúdo vive em
build_deck.py e é aplicado sobre o arquivo existente. Assim uma correção de
número não exige refazer o layout.
"""

from pptx.dml.color import RGBColor
from pptx.enum.text import PP_ALIGN, MSO_ANCHOR
from pptx.util import Inches, Pt

# ------------------------------------------------------------------- cores
# Retiradas do deck original (as quatro primeiras respondem por 95% do uso).
INK = RGBColor(0x1A, 0x1B, 0x3A)   # azul-marinho dos títulos
BODY = RGBColor(0x5A, 0x5F, 0x73)  # cinza-azulado do corpo
MUTED = RGBColor(0x8A, 0x8F, 0xA3)  # notas de rodapé
FAINT = RGBColor(0x9B, 0xA0, 0xBC)
ACCENT = RGBColor(0xF0, 0xA5, 0x00)  # dourado — o acento da marca
MIDNAVY = RGBColor(0x3E, 0x47, 0x80)
WASH = RGBColor(0xF2, 0xF3, 0xF7)  # fundo de caixa
LINE = RGBColor(0xC6, 0xC9, 0xDB)
WHITE = RGBColor(0xFF, 0xFF, 0xFF)

# Cores semânticas de desfecho. Introduzidas só onde há resultado: aqui a cor
# é informação, não decoração. O âmbar de BLOCKED é o próprio acento da marca,
# que já significa "atenção".
OK = RGBColor(0x2E, 0x7D, 0x4F)
NEUTRAL = BODY
BLOCKED = RGBColor(0xC0, 0x86, 0x0A)
BAD = RGBColor(0xB0, 0x3A, 0x2E)

# ------------------------------------------------------------------ fontes
TITLE_FONT = "Cambria"
BODY_FONT = "Calibri"

# --------------------------------------------------------------- geometria
SLIDE_W, SLIDE_H = Inches(13.333), Inches(7.5)
MARGIN = Inches(0.65)
CONTENT_W = Inches(12.03)


def textbox(slide, left, top, width, height):
    box = slide.shapes.add_textbox(left, top, width, height)
    tf = box.text_frame
    tf.word_wrap = True
    tf.margin_left = tf.margin_right = tf.margin_top = tf.margin_bottom = 0
    return box, tf


def style(run, *, size, color, bold=False, font=BODY_FONT, italic=False):
    run.font.size = Pt(size)
    run.font.color.rgb = color
    run.font.bold = bold
    run.font.italic = italic
    run.font.name = font


def title(slide, text, sub=None):
    """Título do slide, na mesma posição e corpo do deck original."""
    _, tf = textbox(slide, MARGIN, Inches(0.45), CONTENT_W, Inches(0.75))
    style(tf.paragraphs[0].add_run(), size=34, color=INK, bold=True, font=TITLE_FONT)
    tf.paragraphs[0].runs[0].text = text
    if sub:
        _, sf = textbox(slide, MARGIN, Inches(1.16), CONTENT_W, Inches(0.34))
        r = sf.paragraphs[0].add_run()
        r.text = sub
        style(r, size=13, color=MUTED)
    return Inches(1.62) if sub else Inches(1.34)


def footnote(slide, text):
    _, tf = textbox(slide, Inches(0.65), Inches(6.95), Inches(12.03), Inches(0.3))
    r = tf.paragraphs[0].add_run()
    r.text = text
    style(r, size=10.5, color=MUTED)


def badge(slide, top, label, *, fill=INK, size=Inches(0.42), left=MARGIN):
    """Quadradinho numerado, como nos slides 'Justificativa' do original."""
    from pptx.enum.shapes import MSO_SHAPE
    sq = slide.shapes.add_shape(MSO_SHAPE.ROUNDED_RECTANGLE, left, top, size, size)
    sq.fill.solid()
    sq.fill.fore_color.rgb = fill
    sq.line.fill.background()
    sq.adjustments[0] = 0.12
    tf = sq.text_frame
    tf.margin_left = tf.margin_right = tf.margin_top = tf.margin_bottom = 0
    tf.vertical_anchor = MSO_ANCHOR.MIDDLE
    p = tf.paragraphs[0]
    p.alignment = PP_ALIGN.CENTER
    r = p.add_run()
    r.text = label
    style(r, size=13, color=WHITE, bold=True)
    return sq


def item(slide, top, num, head, body, *, width=Inches(11.2), left=Inches(1.38),
         badge_fill=INK):
    """Um item numerado: badge + título + corpo. Devolve o topo seguinte."""
    badge(slide, top + Inches(0.05), num, fill=badge_fill)
    _, hf = textbox(slide, left, top, width, Inches(0.35))
    r = hf.paragraphs[0].add_run()
    r.text = head
    style(r, size=16, color=INK, bold=True, font=TITLE_FONT)
    if body:
        _, bf = textbox(slide, left, top + Inches(0.36), width, Inches(0.6))
        r = bf.paragraphs[0].add_run()
        r.text = body
        style(r, size=12.5, color=BODY)
        bf.paragraphs[0].line_spacing = 1.15
        return top + Inches(0.36) + Inches(0.28 * (1 + len(body) // 145))
    return top + Inches(0.5)


def bullets(slide, left, top, width, lines, *, size=12.5, gap=Inches(0.34),
            color=BODY, bullet_color=ACCENT):
    """Lista com marcador losango, sem depender de numeração automática."""
    from pptx.enum.shapes import MSO_SHAPE
    y = top
    for line in lines:
        d = slide.shapes.add_shape(MSO_SHAPE.DIAMOND, left, y + Inches(0.055),
                                   Inches(0.075), Inches(0.075))
        d.fill.solid()
        d.fill.fore_color.rgb = bullet_color
        d.line.fill.background()
        _, tf = textbox(slide, left + Inches(0.2), y, width - Inches(0.2), Inches(0.3))
        p = tf.paragraphs[0]
        p.line_spacing = 1.2
        # **negrito** dentro da linha
        for i, part in enumerate(line.split("**")):
            if not part:
                continue
            r = p.add_run()
            r.text = part
            style(r, size=size, color=INK if i % 2 else color, bold=bool(i % 2))
        y += gap + Inches(0.26 * (len(line) // 110))
    return y


def panel(slide, left, top, width, height, *, fill=WASH, border=None):
    from pptx.enum.shapes import MSO_SHAPE
    box = slide.shapes.add_shape(MSO_SHAPE.ROUNDED_RECTANGLE, left, top, width, height)
    box.fill.solid()
    box.fill.fore_color.rgb = fill
    if border:
        box.line.color.rgb = border
        box.line.width = Pt(1)
    else:
        box.line.fill.background()
    box.adjustments[0] = 0.04
    box.shadow.inherit = False
    return box


def label(slide, left, top, text, *, color=ACCENT, size=10.5, width=None):
    # A largura precisa caber no slide: um rótulo largo demais transborda sem
    # aparecer, porque o texto é curto — some na revisão visual e reprova na
    # verificação automática.
    if width is None:
        width = min(Inches(5), SLIDE_W - left - Inches(0.4))
    _, tf = textbox(slide, left, top, width, Inches(0.24))
    r = tf.paragraphs[0].add_run()
    r.text = text.upper()
    style(r, size=size, color=color, bold=True)
    r.font._rPr.set("spc", "120")  # letter-spacing
