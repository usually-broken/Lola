# Lola

**Most tools make you bend to them. Lola bends to you.**

You can have the same agent as everyone else, or you can have one that does what you
actually need — and nothing you don't. That choice is the whole idea.

## Why it's built this way

Every part of Lola is swappable. The tools, the model, the interface, the loop that drives
it — they're pieces, and you decide which pieces exist. Not which features are switched on
in someone else's product: which parts the program is even made of.

So Lola can be a coding agent, a research assistant, a chat window, or something nobody has
thought of yet. Same program. Yours.

Most harnesses are the same for everyone. Yours isn't.

## Where the shape comes from

Built on the idea from [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness):
everything a plugin, down to the agent loop, and an interface as a frame with places to put
things. That's TypeScript on Cordis. This is Go, written from scratch, and only the shape is
borrowed.

## Install

```sh
brew install usually-broken/tap/lola
```

On Windows, with [Scoop](https://scoop.sh):

```powershell
scoop bucket add usually-broken https://github.com/usually-broken/scoop-bucket
scoop install lola
```

Then:

```sh
lola --frontend web
```

Open <http://127.0.0.1:8765>. It listens on your machine only, and talks to whatever model
you point it at — local or hosted.

## Yours, not a stock install

Everything Lola can do appears in one screen, with a switch beside it. Turn things off
until what's left is exactly what you want. Turn more on when you need them.

That's not a settings menu. That's the product.

## Source

```sh
git clone https://github.com/usually-broken/Lola.git
cd Lola
make install
```

Go 1.27 or newer. `make check` runs what CI runs. `make update` replaces the binary and
never touches your config.

## Licence

Apache 2.0. See `LICENSE` and `NOTICE`.
