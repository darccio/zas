# Zingy

Most. Zen. Static. Website. Generator. Ever.

## Why another one? C'mon, you must be kidding...

I just wanted to set up a very simple website (just a few pages) with Jekyll and it didn't feel right. I didn't want a blog.

I checked other projects but they were incomplete, cumbersome or solved the wrong problem (blogs, blogs everywhere). I wanted a zen-like experience. Just a layout and some Markdown files as pages with unobstrusive structure and configuration.

Yes, it is another NIH but... It is a different kind of beast. I probably overlooked some projects but I find no one that satisfied my requirements.

### Where is the difference?

1. Gophers. There is [Hastie](https://github.com/mkaz/hastie) too. If you want a blog.
2. Markdown only. I really like [Mou](http://mouapp.com/).
3. Just a loop. Zingy just loops over all .md and .html files (oh, I told a little lie in #2, my bad - please, bear with me) in current directory (and subdirectories), ignoring all any other file (including dot-files).
4. Your imagination as limit. I plan to add a simple extension mechanism based in subcommands. Do you really need to handle a blog with zingy? Install/create a new extension and do it!
5. Unobstrutive structure, no '_' files. More in [Usage](#usage) section.

## Usage

Just do:

    $ zingy init

A .zingy directory will be created with sane defaults. Put your layout in .zingy/layout.html and you are ready.

    $ zingy

Yes. Enough. Your delightful site is on .zingy/deploy. Enjoy.

## Configuration and extension

Zingy is like water. It can flow or it can cr... Nevermind, Zingy doesn't crash (please fill an issue if it does).

Everything is configurable at .zingy/config.yml. It is created with default vaules everytime you create a repository.

To extend Zingy functionality you can use and create plugins. Developers, you can develop them in any language (not only in Golang) thanks to Unix magic. And more gophers.

### Plugins

Any prefixed by "zng" (not related to [this](http://en.wikipedia.org/wiki/Croatian_National_Guard)) binary in your [path](http://en.wikipedia.org/wiki/PATH_(variable\)) is a potential Zingy plugin.

All plugins are treated as Zingy subcommands.

For example, we invoke an imaginary plugin called 'znghello' as subcommand:

    $ zingy hello
    Hello!
    
    $ zingy hello World
    Hello World!

That's all. Any command line argument after subcommand name is passed to "znghello" command.

If you develop a new plugin, please contact me and I will list it here :) Please, keep in mind: make it [idempotent](http://en.wikipedia.org/wiki/Idempotence).

## Building sites

Your site layout will look like this:

    $ ls
    $

Just kidding. A normal site would be:

    $ ls -laR
    total 8
    drwxr-xr-x   5 Dario  staff   170 30 mar 16:18 .
    drwxr-xr-x   6 Dario  staff   204 30 mar 13:17 ..
    drwxr-xr-x  13 Dario  staff   442 27 mar 20:05 .git
    drwxr-xr-x   3 Dario  staff   102 30 mar 13:18 .zingy
    -rw-r--r--   1 Dario  staff   941 30 mar 16:19 about.md
    -rw-r--r--@  1 Dario  staff  1645 30 mar 15:31 index.md
    drwxr-xr-x   4 Dario  staff   136 30 mar 16:20 section
    
    [...]
    
    ./.zingy:
    total 0
    drwxr-xr-x  4 Dario  staff   136 30 mar 16:22 .
    drwxr-xr-x  7 Dario  staff   238 30 mar 16:19 ..
    -rw-r--r--  1 Dario  staff    29 30 mar 13:18 config.yml
    -rw-r--r--  1 Dario  staff  2438 30 mar 16:22 layout.html
    
    ./section:
    total 0
    drwxr-xr-x  4 Dario  staff  136 30 mar 16:20 .
    drwxr-xr-x  7 Dario  staff  238 30 mar 16:19 ..
    -rw-r--r--  1 Dario  staff  718 30 mar 16:19 index.md
    -rw-r--r--  1 Dario  staff  991 30 mar 16:20 more.md

All .md files will be converted to HTML and copied in .zingy/deploy using .zingy/layout.html as layout and copying any other files and their structure.

This is also true for .html files.

**Beware**: Zingy won't generate menus for your site. Only .html files.

### What about layout.html?

It is plain HTML. No frills. Just add a placeholder {{.Zingy.Body}} in your template.

First header level 1 from Markdown files will be made available as {{.Zingy.Title}}.

### But... I want to do pages beyond post-like format

No problem! Just use our old friend \<embed\>. Imagine \<layout\> is a valid tag.

    <layout>
    	<nav>
    	    <embed src="navigation.md" type="text/markdown" />
    	</nav>
    	<article>
	    	<embed src="section/index.md" type="text/markdown" />
	    </article>
    </layout>

What does it mean? It means you can have .html files with embedded markdown files.

They will be rendered replacing embed tag if and only if they have type attribute set as "text/markdown".

## Roadmap

With no priority or deadline:

1. Pre and post-commands to generation. Imagine chained subcommands configured to execute before or after generating your site.
2. Metadata in Markdown files as YAML on HTML comments.
3. Feel free to ask if you think Zingy should do something specific in its core.

## Contact me

If I can help you, you have an idea or you are using Zingy in your projects, don't hesitate to drop me a line (or a pull request): [@im_dario](https://twitter.com/im_dario)

## About

Written by [Dario Castañé](http://dario.im).

## License

Zingy is under [GPL v3](http://www.gnu.org/licenses/gpl.html) license.