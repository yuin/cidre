{{/* extends layout_main */}}

<nav>
  <a onclick="newPage();" href="#">New Page</a>
</nav>

<h2> List pages </h2>
{{ if eq (len .Data) 0 }}
  No pages found
{{ else }}
  <ul>
  {{ $app := .App }}
  {{ range $index, $article := .Data }}
    <li><a href="{{ $app.BuildUrl "show_page" $article.Name }}">{{ $article.Name }}</a></li>
  {{ end }}
  </ul>
{{ end }}
