{{/* extends layout_main */}}

<nav>
    <a onclick="newPage();" href="#">New Page</a>
    <a href="{{ .App.BuildUrl "edit_page" .Data.Name }}">Edit this page</a>
</nav>

{{ with .Data }}
<article>
<h1> {{ .Name }} </h1>
<div>
  {{ .Body | raw }} 
</div>
</article>
{{ end }}
