{{/* extends layout_main */}}

<h2> {{.Data.Name }} </h2>
<form action="{{ .App.BuildUrl "save_page" .Data.Name }}" method="POST">
  <fieldset>
    <textarea name="body" cols="100" rows="20">{{ .Data.Body }}</textarea><br />
    <input type="submit" value="submit" />
  </fieldset>
</form>
