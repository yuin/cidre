{{/* extends layout_main */}}

<script>
function deletePage() {
  var form = document.createElement('form');
  form.action = '{{ .App.BuildUrl "delete_page" .Data.Name }}';
  form.method = 'POST';
  document.body.appendChild(form);
  var hidden = document.createElement("input");
  hidden.type = "hidden";
  hidden.name = "_method";
  hidden.value = "delete";
  form.appendChild(hidden);
  form.submit();
}
</script>

<nav class="row">
    <a onclick="newPage();" href="#">New Page</a>
    <a href="{{ .App.BuildUrl "edit_page" .Data.Name }}">Edit this page</a>
    <a onclick="if(confirm('Are you sure?')){deletePage();};" href="#">Delete this page</a>
</nav>

{{ with .Data }}
<article>
<h1> {{ .Name }} </h1>
<div>
  {{ .Body | nl2br }} 
</div>
</article>
{{ end }}
