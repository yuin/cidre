<!DOCTYPE html>
<html lang="ja-JP">
  <head>
    <meta charset="utf-8">
    <title>{{ .Title }}</title>
    <meta name="viewport" content="width=device-width">
    <link rel="stylesheet" href="{{ .App.BuildUrl "statics" "app.css" }}" />
    <!--[if lt IE 9]>
      <script src="http://html5shiv.googlecode.com/svn/trunk/html5.js"></script>
    <![endif]-->
    <script>
      function newPage() {
        var name = prompt("New page name");
        if(name && name.length > 0) {
          location.href = '{{ .App.BuildUrl "edit_page" "{Name}"}}'.replace('{Name}', name);
        }
        return false;
      }
    </script>
  </head>
  <body>
  <div id="wrapper">
    <header role="banner">
    <h1><a href="/" rel="home">{{ .Config.SiteName }}</a></h1>
        <h2>{{ .Config.SiteDescription }}</h2>
    </header>
    <div role="main">
      <div class="flash">
      {{ range $category, $messages := .Flashes }}
        <ul class="flash-{{ $category }}">
            {{ range $index, $message := $messages }}
              <li>{{ $message }}</li>
            {{ end }}
        </ul>
      {{ end }}
      </div>

      {{ yield }}

    </div>
    <footer>
       <p>&copy; Yusuke Inuzuka </p>
    </footer>
  </div>
</body>
</html>
