{{/* extends layout_main */}}

<form action="{{ .App.BuildUrl "save_page" .Data.Name }}" method="POST">
<label for="body">Body:</label>
<textarea name="body">{{ .Data.Body }}</textarea>
<input type="submit" value="submit" />
</form>
